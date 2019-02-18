package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/SermoDigital/jose/jws"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/aggregator"
	agg_v1 "github.com/atlassian/voyager/pkg/apis/aggregator/v1"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/apiservice"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/transport"
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
)

const (
	currentAPIVersion = "v1"
	regionParam       = "region"
	environmentParam  = "envType"
	uriParam          = "uri"

	metricsNamespace = "aggregator"

	specRoute      = "/openapi"
	aggregateRoute = "/aggregate"
)

var (
	apiRoot = path.Join("/", "apis", aggregator.GroupName, currentAPIVersion)
)

type API struct {
	logger *zap.Logger

	router      *chi.Mux
	informer    cache.SharedIndexInformer
	location    voyager.Location
	apiSpecFile string

	asapConfig pkiutil.ASAP

	requestDuration *prometheus.HistogramVec
	requestCounter  *prometheus.CounterVec

	envWhitelist []string
}

type RequestFilter struct {
	URI         string
	Region      string
	Environment string
}

func (r *API) apiResourceList(w http.ResponseWriter, req *http.Request) {
	log := logz.RetrieveLoggerFromContext(req.Context())
	buf, err := json.Marshal(&meta_v1.APIResourceList{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: meta_v1.SchemeGroupVersion.String(),
		},
		GroupVersion: aggregator.GroupName + "/v1",
		APIResources: []meta_v1.APIResource{},
	})
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to marshal api resource list", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httputil.WriteOkResponse(log, w, buf)
}

func NewAPI(logger *zap.Logger, router *chi.Mux, clusterInformer cache.SharedIndexInformer,
	asapConfig pkiutil.ASAP, location voyager.Location, apiFile string, registry prometheus.Registerer, envWhitelist []string) (*API, error) {

	labels := []string{"status", "method", "region", "environment", "path"}

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
		informer:    clusterInformer,
		router:      router,
		location:    location,
		apiSpecFile: apiFile,

		asapConfig: asapConfig,

		requestDuration: requestDuration,
		requestCounter:  requestCounter,

		envWhitelist: envWhitelist,
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
	router.Get(aggregateRoute, r.aggregate)

	router.Get(specRoute, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, req, r.apiSpecFile)
	})
}

func (r *API) aggregate(w http.ResponseWriter, req *http.Request) {
	log := logz.RetrieveLoggerFromContext(req.Context())
	requestFilter, err := r.parseFilters(req)
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to parse filters", err)
		return
	}

	if requestFilter.URI == "" {
		apiservice.RespondWithInternalError(log, w, req, "Request missing uri", err)
		return
	}

	clusters, err := r.getClusters(requestFilter, log)
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to list clusters", err)
		return
	}

	response, err := r.requestClusters(req.Context(), clusters, requestFilter.URI)
	if err != nil {
		apiservice.RespondWithInternalError(log, w, req, "Failed to reach clusters", err)
		return
	}

	j, err := json.Marshal(response)
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

		region := req.URL.Query().Get(regionParam)
		env := req.URL.Query().Get(environmentParam)
		path := req.URL.Path

		httpStatus := strconv.Itoa(m.Code)
		r.requestDuration.WithLabelValues(httpStatus, req.Method, region, env, path).Observe(time.Since(start).Seconds())
		r.requestCounter.WithLabelValues(httpStatus, req.Method, region, env, path).Inc()
	})
}

func (r *API) parseFilters(req *http.Request) (RequestFilter, error) {
	query := req.URL.Query()
	filter := RequestFilter{
		Region:      query.Get(regionParam),
		Environment: query.Get(environmentParam),
		URI:         query.Get(uriParam),
	}

	return filter, nil
}

func (r *API) getClusters(filter RequestFilter, log *zap.Logger) ([]*cr_v1a1.Cluster, error) {
	clusters, err := r.informer.GetIndexer().ByIndex(ByClusterLabelIndexName, "paas")
	if err != nil {
		return nil, err
	}

	var matching []*cr_v1a1.Cluster

	for _, cluster := range clusters {
		c := cluster.(*cr_v1a1.Cluster)
		if (filter.Region == "" || filter.Region == c.ObjectMeta.Labels["region"]) &&
			(filter.Environment == "" || filter.Environment == c.ObjectMeta.Labels["paas-env"]) &&
			isEnvWhitelisted(c.Labels["paas-env"], r.envWhitelist) {
			matching = append(matching, c)
		}
	}

	return matching, nil

}

func isEnvWhitelisted(env string, whitelist []string) bool {
	for _, allowed := range whitelist {
		if env == allowed {
			return true
		}
	}
	return false
}

func (r *API) requestClusters(ctx context.Context, clusters []*cr_v1a1.Cluster, uri string) (*agg_v1.AggregateList, error) {

	list := &agg_v1.AggregateList{
		Items: make([]agg_v1.Aggregate, len(clusters)),
	}

	var wg sync.WaitGroup

	wg.Add(len(clusters))
	for i, cluster := range clusters {
		go func(i int, cluster *cr_v1a1.Cluster) {
			defer wg.Done()
			defer crash.LogPanicAsJSON()
			list.Items[i] = r.makeRequest(ctx, cluster, uri)
		}(i, cluster)
	}

	wg.Wait()

	return list, nil
}

func (r *API) makeRequest(ctx context.Context, cluster *cr_v1a1.Cluster, uri string) agg_v1.Aggregate {
	logger := logz.RetrieveLoggerFromContext(ctx)
	response := agg_v1.Aggregate{
		Name: cluster.ObjectMeta.Name,
		Location: voyager.Location{
			Region:  voyager.Region(cluster.ObjectMeta.Labels["region"]),
			EnvType: voyager.EnvType(cluster.ObjectMeta.Labels["paas-env"]),
		},
	}

	userInfo, ok := request.UserFrom(ctx)
	if !ok {
		return returnErrorResponse(response, logger, errors.New("auth information missing from context"), http.StatusInternalServerError)
	}

	claims := jws.Claims{}
	claims.Set("ns", "voyager")
	claims.Set("grp", userInfo.GetGroups())

	headerValue, err := r.asapConfig.GenerateTokenWithClaims("atlassian.com/kube/tokenator", userInfo.GetName(), claims)
	if err != nil {
		return returnErrorResponse(response, logger, errors.New("Error setting up asap with provider"), http.StatusInternalServerError)
	}

	req, err := http.NewRequest("GET", cluster.Spec.KubernetesAPIEndpoints.ServerEndpoints[0].ServerAddress, nil)
	if err != nil {
		return returnErrorResponse(response, logger, err, http.StatusInternalServerError)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", headerValue))
	req.URL.Path = uri

	rt, err := transport.New(&transport.Config{
		TLS: transport.TLSConfig{
			CAData: cluster.Spec.KubernetesAPIEndpoints.CABundle,
		},
	})

	if err != nil {
		return returnErrorResponse(response, logger, err, http.StatusInternalServerError)
	}

	httpclient := http.Client{
		Transport: rt,
	}

	res, err := httpclient.Do(req)
	if err != nil {
		return returnErrorResponse(response, logger, err, http.StatusInternalServerError)
	}

	defer util.CloseSilently(res.Body)

	response.StatusCode = res.StatusCode
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		if response.StatusCode == http.StatusOK {
			response.StatusCode = http.StatusUnprocessableEntity
		}
		body = []byte("Failed to unmarshal request")
	}

	if res.StatusCode != http.StatusOK {
		return returnClientErrorResponse(response, logger, errors.Errorf("Got non OK response %d from request with body: %s", res.StatusCode, string(body)), res.StatusCode, true)
	}

	blob := map[string]interface{}{}
	err = json.Unmarshal(body, &blob)
	if err != nil {
		logger.Sugar().Debug("Could not unmarshall response: %s", body)
		return returnErrorResponse(response, logger, errors.New("Failed to unmarshal response"), http.StatusUnprocessableEntity)
	}

	response.Body = blob

	return response
}

func returnErrorResponse(r agg_v1.Aggregate, log *zap.Logger, cause error, code int) agg_v1.Aggregate {
	return returnClientErrorResponse(r, log, cause, code, false)
}

func returnClientErrorResponse(r agg_v1.Aggregate, log *zap.Logger, cause error, code int, client bool) agg_v1.Aggregate {
	// This code is our internal code rather than the response code from the cluster request
	if code == http.StatusInternalServerError && !client {
		log.Error("Cluster request failed", zap.Error(cause))
	} else {
		log.Info("Cluster request failed", zap.Error(cause))
	}

	errMsg := cause.Error()
	r.Error = &errMsg
	r.StatusCode = code

	return r
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
