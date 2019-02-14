package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/voyager/pkg/apis/ops"
	"github.com/atlassian/voyager/pkg/k8s"
	c "github.com/atlassian/voyager/pkg/ops/constants"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	scclient_fake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
)

const successfulProxyMsg = "Successful proxy"
const validProviderName = "ec2provider"
const invalidProviderName = "brunothedog"
const requestId = "some-request-id"
const validNamespace = "voyager--with-label"
const wrongNamespace = "hacker"
const validInstanceID = "uuid"
const invalidInstanceID = "invalid-uuid"

func validNamespaceRequestURI() string {
	return path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion, c.Namespaces,
		validNamespace, c.Providers, "ec2provider", c.V2, c.ServiceInstances, validInstanceID, "x-operation_instances", "scale")
}

// Valid URL with a wrong namespace
func wrongNamespaceRequestURI() string {
	return path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion, c.Namespaces,
		wrongNamespace, c.Providers, "ec2provider", c.V2, c.ServiceInstances, validInstanceID, "x-operation_instances", "scale")
}

func invalidNamespaceRequestURI() string {
	return path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion, c.Providers, "ec2provider",
		c.ServiceInstances, validInstanceID, "x-operation_instances", "scale")
}

func invalidProviderForOperation() string {
	return path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion, c.Namespaces,
		"voyager", c.Providers, "brunothedog", c.V2, c.ServiceInstances, validInstanceID, "x-operation_instances", "scale")
}

type MockProvider struct {
	name string
}

func (MockProvider) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

type proxyResponse struct {
	Msg string `json:"msg"`
	URI string `json:"uri"`
}

func (MockProvider) ProxyRequest(asapConfig pkiutil.ASAP, w http.ResponseWriter, r *http.Request, uri string) {

	response := proxyResponse{
		Msg: successfulProxyMsg,
		URI: uri,
	}
	responseJson, _ := json.Marshal(response)
	w.Write(responseJson)
}

func (m MockProvider) Request(asapConfig pkiutil.ASAP, r *http.Request, uri string, user string) (*http.Response, error) {
	return nil, nil
}

func (m MockProvider) Name() string {
	return m.name
}

func (m MockProvider) OwnsPlan(string) bool {
	return false
}

func (m MockProvider) ReportAction() string {
	return "info"
}

func MockSvcCatInformer() cache.SharedIndexInformer {
	scClient := MockSvcCatClient()
	scInf := sc_v1b1inf.NewServiceInstanceInformer(scClient, "", 0, cache.Indexers{
		ServiceInstanceExternalIDIndex: ServiceInstanceExternalIDIndexFunc,
	})
	return scInf
}

func MockSvcCatClient() scClientset.Interface {
	validInstance := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.ServiceInstanceKind,
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: validNamespace,
			Name:      "some-name",
			UID:       "some-uid", // different from externalID
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ExternalID: validInstanceID,
		},
	}
	var obj runtime.Object = &validInstance
	return scclient_fake.NewSimpleClientset(obj)
}

func MockClusterRequest(t *testing.T, requestURI string, providerName string) (*http.Request, *observer.ObservedLogs) {
	req, err := http.NewRequest(
		http.MethodPost,
		requestURI,
		nil,
	)

	require.NoError(t, err)

	logObserver, logs := observer.New(zap.InfoLevel)

	ctx := logz.CreateContextWithLogger(context.Background(), zap.New(logObserver))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", providerName)

	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.RequestIDKey, requestId)
	ctx = request.WithUser(ctx, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
		Extra: map[string][]string{
			"scopes": {"prod", "us-west"},
		},
	})

	req = req.WithContext(ctx)

	return req, logs
}

func MockNamespaceRequest(t *testing.T, requestURI string, namespace string, providerName string) (*http.Request, *observer.ObservedLogs) {
	req, err := http.NewRequest(
		http.MethodPost,
		requestURI,
		nil,
	)

	require.NoError(t, err)

	logObserver, logs := observer.New(zap.InfoLevel)

	ctx := logz.CreateContextWithLogger(context.Background(), zap.New(logObserver))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("instance_id", validInstanceID)
	rctx.URLParams.Add("namespace", namespace)
	rctx.URLParams.Add("provider", providerName)

	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.RequestIDKey, requestId)
	ctx = request.WithUser(ctx, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
		Extra: map[string][]string{
			"scopes": {"prod", "us-west"},
		},
	})

	req = req.WithContext(ctx)

	return req, logs
}

func TestSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	stgr, _ := createOpsAPI(t, router)
	defer stgr.Shutdown()

	handler := router.Middlewares().Handler(loggerMiddleware(http.NotFoundHandler()))
	recorder := httptest.NewRecorder()
	req, _ := MockNamespaceRequest(t, validNamespaceRequestURI(), validNamespace, validProviderName)

	handler.ServeHTTP(recorder, req)

	// Not Found is the successfuly result
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestProxyRequestForOperation(t *testing.T) {
	t.Parallel()

	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	req, _ := MockNamespaceRequest(t, validNamespaceRequestURI(), validNamespace, validProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	proxyRes := proxyResponse{}
	require.NoError(t, json.Unmarshal(body, &proxyRes))

	require.Equal(t, successfulProxyMsg, proxyRes.Msg)
	require.Equal(t, "/v2/service_instances/uuid/x-operation_instances/scale", proxyRes.URI)
	require.Equal(t, http.StatusOK, recorder.Code)

	// throws http = 400 for invalid URI (namespace is mising)
	recorder = httptest.NewRecorder()

	req, _ = MockNamespaceRequest(t, invalidNamespaceRequestURI(), "", validProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	// throws http = 404 for invalid provider
	recorder = httptest.NewRecorder()

	req, _ = MockNamespaceRequest(t, invalidProviderForOperation(), validNamespace, invalidProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestProxyRequestForWrongNamespace(t *testing.T) {
	t.Parallel()

	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	// use a wrong namespace emulating a malicious user trying to invoke operation
	// on instanceID from a different namespace
	req, _ := MockNamespaceRequest(t, wrongNamespaceRequestURI(), wrongNamespace, validProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	stringRes := string(body)
	_ = stringRes
	errorStatus := meta_v1.Status{}
	require.NoError(t, json.Unmarshal(body, &errorStatus))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "some-request-id: Invalid namespaced request: ServiceInstance belongs to a different namespace, requested: hacker, actual: voyager--with-label", errorStatus.Message)
}

func TestProxyRequestForJobs(t *testing.T) {
	t.Parallel()
	validRequestURI := path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion,
		c.Namespaces, validNamespace, c.Providers, "ec2provider", c.V2, c.ServiceInstances,
		validInstanceID, "x-job_instances", "scale_uuid")

	invalidRequestURI := path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion,
		c.Providers, "ec2provider", c.ServiceInstances, validInstanceID, "x-job_instances", "scale_uuid")

	invalidProviderForJob := path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion,
		c.Namespaces, validNamespace, c.Providers, "brunothedog", c.V2, c.ServiceInstances,
		validInstanceID, "x-job_instances", "scale_uuid")

	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	req, _ := MockNamespaceRequest(t, validRequestURI, validNamespace, validProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	proxyRes := proxyResponse{}
	require.NoError(t, json.Unmarshal(body, &proxyRes))

	require.Equal(t, successfulProxyMsg, proxyRes.Msg)
	require.Equal(t, "/v2/service_instances/uuid/x-job_instances/scale_uuid", proxyRes.URI)
	require.Equal(t, http.StatusOK, recorder.Code)

	// throws http = 400 for invalid URI (namespace is missing)
	recorder = httptest.NewRecorder()

	req, _ = MockNamespaceRequest(t, invalidRequestURI, "", validProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	// throws http = 404 for invalid provider
	recorder = httptest.NewRecorder()

	req, _ = MockNamespaceRequest(t, invalidProviderForJob, validNamespace, invalidProviderName)
	opsAPI.proxyNamespaceRequest(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestProxyRequestToListOperations(t *testing.T) {
	t.Parallel()
	validRequestURI := fmt.Sprintf("/apis/%s/%s/clusterproviders/%s/v2/x-operations",
		ops.GatewayGroupName, c.CurrentAPIVersion, validProviderName,
	)

	invalidRequestURI := fmt.Sprintf("/apis/%s/%s/clusterproviders/%s/v2/foooperations",
		ops.GatewayGroupName, c.CurrentAPIVersion, validProviderName,
	)

	invalidProvider := fmt.Sprintf("/apis/%s/%s/clusterproviders/%s/v2/x-operations",
		ops.GatewayGroupName, c.CurrentAPIVersion, invalidProviderName,
	)

	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	req, _ := MockClusterRequest(t, validRequestURI, validProviderName)
	opsAPI.proxyClusterRequest(recorder, req)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	proxyRes := proxyResponse{}
	require.NoError(t, json.Unmarshal(body, &proxyRes))

	require.Equal(t, successfulProxyMsg, proxyRes.Msg)
	require.Equal(t, "/v2/x-operations", proxyRes.URI)
	require.Equal(t, http.StatusOK, recorder.Code)

	// invalid URI (route not configured), return the http code returned by the remote server.
	recorder = httptest.NewRecorder()

	req, _ = MockClusterRequest(t, invalidRequestURI, validProviderName)
	opsAPI.proxyClusterRequest(recorder, req)

	body, err = ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	proxyRes = proxyResponse{}
	require.NoError(t, json.Unmarshal(body, &proxyRes))

	// yes, we are checking if the status is 200 because its a proxy request
	// and we return the status code returned by the provider.
	require.Equal(t, "/v2/foooperations", proxyRes.URI)
	require.Equal(t, http.StatusOK, recorder.Code)

	// throws http = 404 for invalid provider
	recorder = httptest.NewRecorder()

	req, _ = MockClusterRequest(t, invalidProvider, invalidProvider)
	opsAPI.proxyClusterRequest(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestProxyMissingRequest(t *testing.T) {
	t.Parallel()
	requestURI := path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion,
		c.Namespaces, validNamespace, c.Providers, "ec2provider", c.V2, c.ServiceInstances,
		validInstanceID, "x-operation_instances", "scale")

	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "other-proxy"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	req, _ := MockNamespaceRequest(t, requestURI, validNamespace, validProviderName)
	opsAPI.proxyClusterRequest(recorder, req)

	require.Equal(t, http.StatusNotFound, recorder.Code)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)

	opsErrors := meta_v1.Status{}
	require.NoError(t, json.Unmarshal(body, &opsErrors))
	require.Equal(t, opsErrors.Message, "some-request-id: Provider `ec2provider` does not exist")
	require.EqualValues(t, opsErrors.Code, http.StatusNotFound)
}

func TestGetProvidersRequest(t *testing.T) {
	t.Parallel()

	// return httpcode=200 and list of providers.
	stgr, opsAPI := createOpsAPI(t, chi.NewRouter())
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder := httptest.NewRecorder()
	req, _ := mockProvidersRequest(t)

	opsAPI.getProviderNames(recorder, req)

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	require.Equal(t, "{\"ProviderNames\":[\"ec2provider\"]}\n", string(body))
	require.Equal(t, http.StatusOK, recorder.Code)

	// return httpcode=200 and empty providers if there are no providers.
	opsAPI.RemoveProvider("ec2provider")
	recorder = httptest.NewRecorder()
	opsAPI.getProviderNames(recorder, req)
	body, err = ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	require.Equal(t, "{\"ProviderNames\":[]}\n", string(body))
	require.Equal(t, http.StatusOK, recorder.Code)
}

func mockProvidersRequest(t *testing.T) (*http.Request, *observer.ObservedLogs) {
	req, err := http.NewRequest(
		http.MethodGet,
		path.Join("/", c.Apis, ops.GatewayGroupName, c.CurrentAPIVersion, c.ClusterProviders),
		nil,
	)

	require.NoError(t, err)
	logObserver, logs := observer.New(zap.InfoLevel)
	ctx := logz.CreateContextWithLogger(context.Background(), zap.New(logObserver))

	rctx := chi.NewRouteContext()
	ctx = request.WithUser(ctx, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
	})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	return req, logs
}

func TestPrometheusInstrumentation(t *testing.T) {
	t.Parallel()
	rtr := chi.NewRouter()
	stgr, opsAPI := createOpsAPI(t, rtr)
	defer stgr.Shutdown()

	mockProvider := MockProvider{name: "ec2provider"}
	opsAPI.AddOrUpdateProvider(mockProvider)

	recorder1 := httptest.NewRecorder()
	req1, _ := mockProvidersRequest(t)
	rtr.ServeHTTP(recorder1, req1)

	req2, _ := mockProvidersRequest(t)
	recorder2 := httptest.NewRecorder()
	rtr.ServeHTTP(recorder2, req2)

	body, err := ioutil.ReadAll(recorder1.Result().Body)
	require.NoError(t, err)
	require.Equal(t, "{\"ProviderNames\":[\"ec2provider\"]}\n", string(body))
	require.Equal(t, http.StatusOK, recorder1.Code)

	body, err = ioutil.ReadAll(recorder2.Result().Body)
	require.NoError(t, err)
	require.Equal(t, "{\"ProviderNames\":[\"ec2provider\"]}\n", string(body))
	require.Equal(t, http.StatusOK, recorder2.Code)

	var outputMetric io_prometheus_client.Metric
	opsAPI.requestDuration.WithLabelValues("200", "GET", "ops", "", "").(prometheus.Histogram).Write(&outputMetric)
	opsAPI.requestCounter.WithLabelValues("200", "GET", "ops", "", "").Write(&outputMetric)

	require.Equal(t, uint64(0), *outputMetric.Histogram.Bucket[0].CumulativeCount)
	require.Equal(t, float64(0), *outputMetric.Histogram.Bucket[0].UpperBound)

	require.Equal(t, uint64(2), *outputMetric.Histogram.Bucket[1].CumulativeCount)
	require.Equal(t, float64(0.1), *outputMetric.Histogram.Bucket[1].UpperBound)

	require.Equal(t, float64(2), *outputMetric.Counter.Value)
}

func createOpsAPI(t *testing.T, router *chi.Mux) (stager.Stager, *API) {
	instanceInf := MockSvcCatInformer()

	stgr := stager.New()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	// Start informers then wait on them
	stage.StartWithChannel(instanceInf.Run)
	if !assert.True(t, cache.WaitForCacheSync(ctx.Done(), instanceInf.HasSynced)) {
		stgr.Shutdown()
		assert.FailNow(t, "Timed out waiting for informer sync")
	}

	api, err := NewOpsAPI(zaptest.NewLogger(t), &MockASAPConfig{}, router, prometheus.NewPedanticRegistry(), instanceInf)
	if err != nil {
		stgr.Shutdown()
	}
	require.NoError(t, err)
	return stgr, api
}
