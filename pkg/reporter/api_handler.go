package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"github.com/atlassian/voyager/pkg/apis/reporter"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ops"
	opsZappers "github.com/atlassian/voyager/pkg/ops/util/zappers"
	"github.com/atlassian/voyager/pkg/util/apiservice"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/layers"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
)

const (
	currentAPIVersion = "v1"
	namespacePath     = "namespace"
	servicePath       = "service"

	metricsNamespace = "reporter"

	defaultNamespace = "default" // Default namespace set by kubectl if nothing is set
)

var (
	apiRoot                    = fmt.Sprintf("/apis/%s/%s", reporter.GroupName, currentAPIVersion)
	specRoute                  = "/openapi"
	summaryRoute               = fmt.Sprintf("/summaries/{%s}", servicePath)
	namespaceSummaryRoute      = fmt.Sprintf("/namespaces/{%s}/summaries", namespacePath)
	namespaceNamedSummaryRoute = fmt.Sprintf("/namespaces/{%s}/summaries/{%s}", namespacePath, servicePath)
	reportRoute                = fmt.Sprintf("/reports/{%s}", servicePath)
	namespaceReportRoute       = fmt.Sprintf("/namespaces/{%s}/reports", namespacePath)
	namespaceNamedReportRoute  = fmt.Sprintf("/namespaces/{%s}/reports/{%s}", namespacePath, servicePath)
)

type API struct {
	logger *zap.Logger

	router      *chi.Mux
	informers   map[schema.GroupVersionKind]cache.SharedIndexInformer
	location    voyager.Location
	apiSpecFile string

	providers    map[string]ops.ProviderInterface
	providerLock sync.RWMutex
	asapConfig   pkiutil.ASAP

	requestDuration *prometheus.HistogramVec
	requestCounter  *prometheus.CounterVec
}

type RequestFilter struct {
	Namespace       string
	Service         string
	Layer           string
	Type            string
	Status          string
	Account         string
	Region          string
	EnvironmentType string
}

func setFilter(params map[string][]string, field string, existing string) string {
	if val, ok := params[field]; ok && len(val) > 0 {
		return val[0]
	}

	return existing
}

func (r *RequestFilter) apply(params map[string][]string) {
	r.Layer = setFilter(params, "layer", r.Layer)
	r.Type = setFilter(params, "type", r.Type)
	r.Account = setFilter(params, "account", r.Account)
	r.Region = setFilter(params, "region", r.Region)
	r.EnvironmentType = setFilter(params, "environment_type", r.EnvironmentType)
}

func (r *API) apiResourceList(w http.ResponseWriter, req *http.Request) {
	log := logz.RetrieveLoggerFromContext(req.Context())
	buf, err := json.Marshal(&meta_v1.APIResourceList{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: meta_v1.SchemeGroupVersion.String(),
		},
		GroupVersion: reporter_v1.ReportResourceAPIVersion,
		APIResources: []meta_v1.APIResource{
			{
				Name:         reporter_v1.ReportResourcePlural,
				SingularName: reporter_v1.ReportResourceSingular,
				Namespaced:   true,
				Kind:         reporter_v1.ReportResourceKind,
				Verbs:        []string{"get", "list", "watch"},
			},
			{
				Name:         reporter_v1.SummaryResourcePlural,
				SingularName: reporter_v1.SummaryResourceSingular,
				Namespaced:   true,
				Kind:         reporter_v1.SummaryResourceKind,
				Verbs:        []string{"get", "list", "watch"},
			},
		},
	})
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to marshal api resource list", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httputil.WriteOkResponse(log, w, buf)
}

func (r *API) kubifyResponse(ctx context.Context, reports []*NamespaceReportHandler, expand *string) reporter_v1.ReportList {
	resp := reporter_v1.ReportList{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ReportList",
			APIVersion: reporter_v1.ReportResourceAPIVersion,
		},
		ListMeta: meta_v1.ListMeta{},
		Items:    make([]reporter_v1.Report, len(reports)),
	}

	providers := func() map[string]ops.ProviderInterface {
		r.providerLock.RLock()
		defer r.providerLock.RUnlock()

		resp := make(map[string]ops.ProviderInterface, len(r.providers))
		for k, v := range r.providers {
			resp[k] = v
		}
		return resp
	}()

	var wg sync.WaitGroup

	wg.Add(len(reports))
	for i, report := range reports {
		go func(i int, report *NamespaceReportHandler) {
			defer wg.Done()
			defer crash.LogPanicAsJSON()
			if expand == nil {
				resp.Items[i] = report.GenerateReport(ctx, providers, r.asapConfig)
			} else {
				resp.Items[i] = report.GenerateSummary(*expand)
			}
		}(i, report)
	}

	wg.Wait()

	for i := range resp.Items {
		// TODO can we label ns with label?
		resp.Items[i].Location = r.location
	}

	return resp
}

func NewReportingAPI(logger *zap.Logger, router *chi.Mux, informers map[schema.GroupVersionKind]cache.SharedIndexInformer,
	asapConfig pkiutil.ASAP, location voyager.Location, apiFile string, registry prometheus.Registerer) (*API, error) {

	labels := []string{"status", "method", "service", "namespace", "path"}

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "request_duration_seconds",
			Help:      "Time taken to process a request",
			Buckets:   prometheus.LinearBuckets(0, 0.1, 10),
		},
		labels,
	)

	requestCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "requests_total",
			Help:      "Total number of requests to OPS Gateway",
		},
		labels,
	)

	if err := registry.Register(requestCounter); err != nil {
		return nil, err
	}
	if err := registry.Register(requestDuration); err != nil {
		return nil, err
	}

	r := &API{
		logger:      logger,
		informers:   informers,
		router:      router,
		location:    location,
		apiSpecFile: apiFile,

		asapConfig: asapConfig,
		providers:  map[string]ops.ProviderInterface{},

		requestDuration: requestDuration,
		requestCounter:  requestCounter,
	}

	r.register(router)
	return r, nil
}

func (r *API) register(router *chi.Mux) {
	apiRouter := chi.NewRouter()
	r.registerServiceInstances(apiRouter)
	router.Mount(apiRoot, apiRouter)
}

func (r *API) registerServiceInstances(router *chi.Mux) {
	router.Use(loggerMiddleware, r.Instrumentation)
	router.Get("/", r.apiResourceList)
	router.Get(reportRoute, r.generateReport)
	router.Get(namespaceReportRoute, r.generateReport)
	router.Get(namespaceNamedReportRoute, r.generateReport)
	router.Get(summaryRoute, r.generateSummary)
	router.Get(namespaceSummaryRoute, r.generateSummary)
	router.Get(namespaceNamedSummaryRoute, r.generateSummary)

	router.Get(specRoute, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, req, r.apiSpecFile)
	})
}

func (r *API) AddOrUpdateProvider(p ops.ProviderInterface) {
	r.providerLock.Lock()
	defer r.providerLock.Unlock()

	r.logger.Info("Provider acknowledged", opsZappers.ProviderName(p.Name()))
	r.providers[p.Name()] = p
}

func (r *API) getNamespaces(service string) ([]*core_v1.Namespace, error) {
	nsInf := r.informers[core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind)]
	namespaces, err := nsInf.GetIndexer().ByIndex(ByServiceNameLabelIndexName, service)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp := make([]*core_v1.Namespace, 0, len(namespaces))
	for _, ns := range namespaces {
		resp = append(resp, ns.(*core_v1.Namespace).DeepCopy())
	}
	return resp, nil
}

func (r *API) getObjects(namespace string, filter RequestFilter) ([]runtime.Object, error) {
	var objs []runtime.Object
	for gvk, inf := range r.informers {
		if gvk == core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind) ||
			gvk == ops_v1.SchemeGroupVersion.WithKind("Route") {
			// We don't want to get namespace or route objects
			continue
		}

		if filter.Layer != "" {
			if _, ok := voyagerGVKs[gvk]; ok {
				if voyagerLayers[filter.Layer] != gvk {
					continue
				}
			} else {
				if filter.Layer != reporter_v1.LayerObject && filter.Layer != reporter_v1.LayerProvider {
					continue
				}
			}
		}
		if gvk == comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind) {
			obj, exists, indexerErr := inf.GetIndexer().GetByKey(namespace)
			if indexerErr != nil {
				return objs, indexerErr
			}
			if exists {
				obj := obj.(runtime.Object).DeepCopyObject()
				obj.GetObjectKind().SetGroupVersionKind(gvk)
				objs = append(objs, obj)
			}
		} else {
			interfaceList, indexerErr := inf.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
			if indexerErr != nil {
				return objs, indexerErr
			}
			for _, o := range interfaceList {
				obj := o.(runtime.Object).DeepCopyObject()
				obj.GetObjectKind().SetGroupVersionKind(gvk)
				objs = append(objs, obj)
			}
		}
	}
	return objs, nil
}

func (r *API) generateNamespaces(filter RequestFilter) ([]*NamespaceReportHandler, error) {
	var ns []*NamespaceReportHandler
	var namespaces []*core_v1.Namespace

	// We also check for the default namespace for requests from kubectl like `kubectl get reports my-service`
	if filter.Namespace != "" && filter.Namespace != defaultNamespace {
		nsInf := r.informers[core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind)]
		n, exists, err := nsInf.GetIndexer().GetByKey(filter.Namespace)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if exists {
			namespaces = append(namespaces, n.(*core_v1.Namespace).DeepCopy())
		}
	} else {
		serviceNamespaces, err := r.getNamespaces(filter.Service)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		namespaces = append(namespaces, serviceNamespaces...)
	}

	for _, n := range namespaces {
		objs, err := r.getObjects(n.Name, filter)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		serviceName, err := layers.ServiceNameFromNamespaceLabels(n.GetLabels())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		nrh, err := NewNamespaceReportHandler(n.Name, serviceName, objs, filter, r.location)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		ns = append(ns, nrh)
	}

	return ns, nil
}

func (r *API) generateSummary(w http.ResponseWriter, req *http.Request) {
	expand := req.URL.Query().Get("expand")
	requestFilter, err := r.parseFilters(req)
	log := logz.RetrieveLoggerFromContext(req.Context())
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to parse filters", err)
		return
	}

	ns, err := r.generateNamespaces(requestFilter)
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to get namespace objects", err)
		return
	}

	j, err := json.Marshal(r.kubifyResponse(req.Context(), ns, &expand))
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to marshal response", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	httputil.WriteOkResponse(log, w, j)
}

func (r *API) generateReport(w http.ResponseWriter, req *http.Request) {
	requestFilter, err := r.parseFilters(req)
	log := logz.RetrieveLoggerFromContext(req.Context())
	if err != nil {
		apiservice.RespondWithBadRequest(log, w, req, "Failed to parse request filters", err)
		return
	}

	ns, err := r.generateNamespaces(requestFilter)
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to retrieve namespace information", err)
		return
	}

	j, err := json.Marshal(r.kubifyResponse(req.Context(), ns, nil))
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to marshal response", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	httputil.WriteOkResponse(log, w, j)
}

func (r *API) Instrumentation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		m := httpsnoop.CaptureMetrics(next, w, req)
		serviceParam := getURLParameter(req, servicePath, "")
		namespaceParam := getURLParameter(req, namespacePath, "all-namespaces")
		path := req.URL.Path

		httpStatus := strconv.Itoa(m.Code)
		r.requestDuration.WithLabelValues(httpStatus, req.Method, serviceParam, namespaceParam, path).Observe(time.Since(start).Seconds())
		r.requestCounter.WithLabelValues(httpStatus, req.Method, serviceParam, namespaceParam, path).Inc()
	})
}

func getURLParameter(r *http.Request, parameterName string, defaultValue string) string {
	param := chi.URLParam(r, parameterName)
	if len(param) == 0 {
		return defaultValue
	}
	return param
}

// route object event handler func's
func (r *API) OnAdd(obj interface{}) {
	route := obj.(*ops_v1.Route)
	_, provider, err := ops.NewProvider(r.logger, route)
	if err != nil {
		r.logger.Error("Failed to setup provider", opsZappers.Route(route), zap.Error(err))
		return
	}

	r.logger.Info("Processed provider", opsZappers.Route(route))
	r.providerLock.Lock()
	defer r.providerLock.Unlock()

	r.providers[provider.Name()] = provider
}

func (r *API) OnUpdate(oldObj, newObj interface{}) {
	r.OnAdd(newObj)
}

func (r *API) OnDelete(obj interface{}) {
	metaObj, ok := obj.(meta_v1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			r.logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		metaObj, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			r.logger.Sugar().Errorf("Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}

	r.logger.Info("Deleting provider", opsZappers.ProviderName(metaObj.GetName()))
	r.providerLock.Lock()
	defer r.providerLock.Unlock()

	delete(r.providers, metaObj.GetName())
}

func (r *API) parseFilters(req *http.Request) (RequestFilter, error) {
	filter := RequestFilter{
		Namespace: chi.URLParam(req, namespacePath),
		Service:   chi.URLParam(req, servicePath),
	}

	queryParams := req.URL.Query()
	filter.apply(queryParams)

	selector, err := parseFieldSelector(queryParams.Get("fieldSelector"))

	if err != nil {
		return filter, err
	}

	filter.apply(selector)

	return filter, nil
}

func parseFieldSelector(fieldSelectorQuery string) (map[string][]string, error) {

	result := map[string][]string{}
	if fieldSelectorQuery == "" {
		return result, nil
	}

	parsedQuery, err := url.QueryUnescape(fieldSelectorQuery)
	if err != nil {
		return result, err
	}

	pairs := strings.Split(parsedQuery, "&")
	for _, v := range pairs {
		keyPair := strings.Split(v, "=")
		if len(keyPair) != 2 {
			return result, errors.Errorf("Invalid field selector %s", v)
		}
		result[keyPair[0]] = []string{keyPair[1]}
	}
	return result, nil
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		logger := logz.RetrieveLoggerFromContext(ctx)

		userInfo, ok := request.UserFrom(ctx)
		if ok {
			ctx = logz.CreateContextWithLogger(ctx, logger.With(
				zap.String("user", userInfo.GetName()),
				zap.Strings("groups", userInfo.GetGroups()),
			))
			newReq := req.WithContext(ctx)
			next.ServeHTTP(w, newReq)
			return
		}

		next.ServeHTTP(w, req)
	})
}
