package ops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/atlassian/voyager/pkg/apis/ops"
	"github.com/atlassian/voyager/pkg/ops/constants"
	"github.com/atlassian/voyager/pkg/ops/util/zappers"
	"github.com/atlassian/voyager/pkg/util/apiservice"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	prom_util "github.com/atlassian/voyager/pkg/util/prometheus"
	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
)

var (
	apiRoot              = path.Join("/", constants.Apis, ops.GatewayGroupName, constants.CurrentAPIVersion)
	providersRoute       = path.Join("/", constants.ClusterProviders)
	listOperationsRoute  = path.Join("/", constants.ClusterProviders, pathParam(constants.ProviderPath), constants.V2, constants.XOperations)
	invokeOperationRoute = path.Join("/", constants.Namespaces, pathParam(constants.NamespacePath), constants.Providers, pathParam(constants.ProviderPath), constants.V2, constants.ServiceInstances, pathParam(constants.InstanceIDPath))
)

type API struct {
	logger       *zap.Logger
	Providers    map[string]ProviderInterface
	ProviderLock sync.RWMutex
	ASAPConfig   pkiutil.ASAP

	instanceInformer cache.SharedIndexInformer

	requestDuration *prometheus.HistogramVec
	requestCounter  *prometheus.CounterVec
}

type ProviderResponse struct {
	ProviderNames []string
}

func (*ProviderResponse) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (Provider) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func (o *API) apiResourceList(w http.ResponseWriter, r *http.Request) {
	buf, err := json.Marshal(&meta_v1.APIResourceList{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: constants.CurrentAPIResourceListVersion,
		},
		GroupVersion: fmt.Sprintf("%s/%s", ops.GatewayGroupName, constants.CurrentAPIVersion),
		APIResources: []meta_v1.APIResource{},
	})
	if err != nil {
		apiservice.RespondWithInternalError(o.logger, w, r, "Failed to marshal api resource list", err)
	} else {
		w.Header().Set("Content-Type", "application/json")
		httputil.WriteOkResponse(o.logger, w, buf)
	}
}

func (o *API) getProviderNames(w http.ResponseWriter, r *http.Request) {
	keys := o.getProviderKeys()
	err := render.Render(w, r, &ProviderResponse{ProviderNames: keys})

	if err != nil {
		apiservice.RespondWithInternalError(o.logger, w, r, "Failed to render", err)
	}
}

func (o *API) getProviderKeys() []string {
	o.ProviderLock.RLock()
	defer o.ProviderLock.RUnlock()

	keys := make([]string, 0, len(o.Providers))

	for k := range o.Providers {
		keys = append(keys, k)
	}

	return keys
}

func (o *API) GetProvider(name string) (ProviderInterface, error) {
	o.ProviderLock.RLock()
	defer o.ProviderLock.RUnlock()
	if provider, ok := o.Providers[name]; ok {
		return provider, nil
	}
	return &Provider{}, errors.Errorf("No provider found named %s", name)
}

func NewOpsAPI(logger *zap.Logger, asapConfig pkiutil.ASAP, router *chi.Mux,
	registry prometheus.Registerer,
	instanceInf cache.SharedIndexInformer) (*API, error) {

	labels := []string{"status", "method", "provider", "operation", "job"}

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: constants.Ops,
			Name:      "request_duration_seconds",
			Help:      "Time taken to process a request",
			Buckets:   prometheus.LinearBuckets(0, 0.1, 10),
		},
		labels,
	)

	requestCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: constants.Ops,
			Name:      "requests_total",
			Help:      "Total number of requests to OPS Gateway",
		},
		labels,
	)

	if err := prom_util.RegisterAll(registry, requestCounter, requestDuration); err != nil {
		return nil, err
	}

	o := &API{
		logger:           logger,
		Providers:        map[string]ProviderInterface{},
		ASAPConfig:       asapConfig,
		requestDuration:  requestDuration,
		requestCounter:   requestCounter,
		instanceInformer: instanceInf,
	}

	err := o.register(router)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (o *API) AddOrUpdateProvider(p ProviderInterface) {
	o.ProviderLock.Lock()
	defer o.ProviderLock.Unlock()

	o.logger.Info("Provider acknowledged", zappers.ProviderName(p.Name()))
	o.Providers[p.Name()] = p
}

func (o *API) RemoveProvider(providerName string) {
	o.ProviderLock.Lock()
	defer o.ProviderLock.Unlock()

	o.logger.Info("Provider removed", zappers.ProviderName(providerName))
	delete(o.Providers, providerName)
}

func (o *API) register(r *chi.Mux) error {
	apiRouter := chi.NewRouter()
	o.registerServiceInstances(apiRouter)
	r.Mount(apiRoot, apiRouter)
	return nil
}

func (o *API) registerServiceInstances(r *chi.Mux) {
	r.Use(loggerMiddleware, o.Instrumentation)
	r.Get("/", o.apiResourceList)
	r.Get(providersRoute, o.getProviderNames)
	r.Get(listOperationsRoute, o.proxyClusterRequest)

	r.Route(invokeOperationRoute, func(r chi.Router) {
		r.HandleFunc("/*", o.proxyNamespaceRequest)
	})
}

func (o *API) proxyClusterRequest(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, constants.ProviderPath)
	provider, err := o.GetProvider(providerName)
	if err != nil {
		apiservice.RespondWithNotFoundError(o.logger, w, r, fmt.Sprintf("Provider `%s` does not exist", providerName), err)
		return
	}

	// reconstruct the URI prefix to extract the URI suffix to be sent in a proxy request
	uriPrefix := path.Join("/", constants.ClusterProviders, providerName)
	o.proxyRequest(w, r, provider, uriPrefix)
}

func (o *API) proxyNamespaceRequest(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, constants.ProviderPath)
	provider, err := o.GetProvider(providerName)
	if err != nil {
		apiservice.RespondWithNotFoundError(o.logger, w, r, fmt.Sprintf("Provider `%s` does not exist", providerName), err)
		return
	}

	err = o.validateNamespaceRequest(r)
	if err != nil {
		apiservice.RespondWithBadRequest(o.logger, w, r, fmt.Sprintf("Invalid namespaced request: %v", err.Error()), err)
		return
	}

	namespace := chi.URLParam(r, constants.NamespacePath)
	// reconstruct the URI prefix to extract the URI suffix to be sent in a proxy request
	uriPrefix := path.Join("/", constants.Namespaces, namespace, constants.Providers, providerName)
	o.proxyRequest(w, r, provider, uriPrefix)
}

func (o *API) validateNamespaceRequest(r *http.Request) error {
	namespace := chi.URLParam(r, constants.NamespacePath)
	if namespace == "" {
		return errors.New("namespace is missing in the request")
	}
	instanceID := chi.URLParam(r, constants.InstanceIDPath)
	if instanceID == "" {
		return errors.New("instanceID is missing in the request")
	}

	instances, err := o.instanceInformer.GetIndexer().ByIndex(ServiceInstanceExternalIDIndex, instanceID)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(instances) == 0 {
		return errors.Errorf("ServiceInstance not found: %v", instanceID)
	}
	if len(instances) > 1 {
		return errors.Errorf("more than one ServiceInstance with a given externalID found: %v (%v)", instanceID, len(instances))
	}
	instance := instances[0].(*sc_v1b1.ServiceInstance)
	if instance.Namespace != namespace {
		return errors.Errorf("ServiceInstance belongs to a different namespace, requested: %v, actual: %v", namespace, instance.Namespace)
	}
	return nil
}

func (o *API) proxyRequest(w http.ResponseWriter, r *http.Request, provider ProviderInterface, uriPrefix string) {
	uriSuffix, err := extractURISuffix(r.URL.Path, uriPrefix)
	if err != nil {
		apiservice.RespondWithInternalError(o.logger, w, r, "Failed to parse URI", err)
		return
	}

	provider.ProxyRequest(o.ASAPConfig, w, r, uriSuffix)
}

func pathParam(name string) string {
	return "{" + name + "}"
}

// Parses the given path to return the URI to call an operation on the broker/provider.
func extractURISuffix(uri, prefix string) (string, error) {
	if strings.Contains(uri, prefix) {
		return strings.Split(uri, prefix)[1], nil
	}

	return "", errors.Errorf("cannot parse URI %s", uri)
}

func (o *API) Instrumentation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m := httpsnoop.CaptureMetrics(next, w, r)

		providerName := getURLParameter(r, constants.ProviderPath, constants.Ops)
		instanceID := getURLParameter(r, constants.InstanceIDPath, "")

		var operationID, jobID string

		if len(instanceID) > 0 {
			if strings.Contains(r.URL.Path, constants.XOperationsInstances) {
				operationID = strings.Split(r.URL.Path, constants.XOperationsInstances+"/")[1]
			} else if strings.Contains(r.URL.Path, constants.XJobsInstances) {
				jobID = strings.Split(r.URL.Path, constants.XJobsInstances+"/")[1]
			}
		}

		httpStatus := strconv.Itoa(m.Code)
		o.requestDuration.WithLabelValues(httpStatus, r.Method, providerName, operationID, jobID).Observe(time.Since(start).Seconds())
		o.requestCounter.WithLabelValues(httpStatus, r.Method, providerName, operationID, jobID).Inc()
	})
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

func getURLParameter(r *http.Request, parameterName string, defaultValue string) string {
	param := chi.URLParam(r, parameterName)
	if len(param) == 0 {
		return defaultValue
	}
	return param
}
