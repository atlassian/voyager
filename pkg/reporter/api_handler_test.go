package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ash2k/stager"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	bundleClient "github.com/atlassian/smith/pkg/client"
	smithclient_fake "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/fake"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/apis/reporter"
	reporter_v1 "github.com/atlassian/voyager/pkg/apis/reporter/v1"
	compclient_fake "github.com/atlassian/voyager/pkg/composition/client/fake"
	compInf "github.com/atlassian/voyager/pkg/composition/informer"
	formclient_fake "github.com/atlassian/voyager/pkg/formation/client/fake"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	"github.com/atlassian/voyager/pkg/k8s"
	orchclient_fake "github.com/atlassian/voyager/pkg/orchestration/client/fake"
	orchInf "github.com/atlassian/voyager/pkg/orchestration/informer"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/go-chi/chi"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scclient_fake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	coreclient_fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestGenerateReport(t *testing.T) {
	t.Parallel()

	locationObject := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd",
			Namespace: "testns",
			UID:       "some-uid",
		},
		Spec: form_v1.LocationDescriptorSpec{
			ConfigMapName: "cm1",
			Resources: []form_v1.LocationDescriptorResource{
				{
					Name: "old-resource",
					Type: "some-type",
				},
			},
		},
	}

	nsObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testns",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "test",
			},
		},
	}

	stateObject := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       orch_v1.StateResourceKind,
			APIVersion: orch_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test",
			Namespace: "testns",
		},
		Spec: orch_v1.StateSpec{
			ConfigMapName: "cm1",
			Resources: []orch_v1.StateResource{
				orch_v1.StateResource{
					Name: "messages",
					Type: "sns",
				},
				orch_v1.StateResource{
					Name: "events",
					Type: "sqs",
					DependsOn: []orch_v1.StateDependency{
						orch_v1.StateDependency{
							Name: "messages",
							Attributes: map[string]interface{}{
								"MaxReceiveCount": "100",
							},
						},
					},
				},
			},
		},
		Status: orch_v1.StateStatus{
			Conditions: []cond_v1.Condition{
				{
					Status: cond_v1.ConditionTrue,
					Type:   cond_v1.ConditionInProgress,
				},
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/apis/%s/%s/namespaces/%s/reports",
		reporter.GroupName, currentAPIVersion, "testns",
	), nil)

	require.NoError(t, err)

	tc := testCase{
		logger: zaptest.NewLogger(t),

		formationObjects:    []runtime.Object{locationObject},
		orchestraionObjects: []runtime.Object{stateObject},
		nsObjects:           []runtime.Object{nsObject},

		request: req,

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)
			require.NoError(t, err)

			reportList := reporter_v1.ReportList{}
			err = json.Unmarshal(body, &reportList)
			require.NoError(t, err)

			require.Equal(t, 1, len(reportList.Items))
			require.Equal(t, "testns", reportList.Items[0].ObjectMeta.Namespace)
			require.Equal(t, "test", reportList.Items[0].ObjectMeta.Name)
			require.Equal(t, 1, len(reportList.Items[0].Report.Formation.Resources))
			require.Equal(t, "old-resource", reportList.Items[0].Report.Formation.Resources[0].Name)
			require.Equal(t, 2, len(reportList.Items[0].Report.Orchestration.Resources))

			orchResources := make([]string, 0, len(reportList.Items[0].Report.Orchestration.Resources))
			for _, r := range reportList.Items[0].Report.Orchestration.Resources {
				orchResources = append(orchResources, r.Name)
			}

			require.ElementsMatch(t, []string{"messages", "events"}, orchResources)
		},
	}

	tc.run(t)
}

func TestFilterService(t *testing.T) {
	t.Parallel()

	nsObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testns",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "test",
			},
		},
	}

	nsOtherObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testnsother",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "testother",
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/apis/%s/%s/reports/%s",
		reporter.GroupName, currentAPIVersion, "test",
	), nil)

	require.NoError(t, err)

	tc := testCase{
		logger: zaptest.NewLogger(t),

		nsObjects: []runtime.Object{nsObject, nsOtherObject},

		request: req,

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)
			require.NoError(t, err)

			reportList := reporter_v1.ReportList{}
			err = json.Unmarshal(body, &reportList)
			require.NoError(t, err)

			require.Equal(t, 1, len(reportList.Items))
			require.Equal(t, "testns", reportList.Items[0].ObjectMeta.Namespace)
			require.Equal(t, "test", reportList.Items[0].ObjectMeta.Name)
		},
	}

	tc.run(t)
}

func TestFilterServiceWithDefaultNamespace(t *testing.T) {
	t.Parallel()

	nsObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testns",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "test",
			},
		},
	}

	nsOtherObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testnsother",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "testother",
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/apis/%s/%s/namespaces/default/reports/%s",
		reporter.GroupName, currentAPIVersion, "test",
	), nil)

	require.NoError(t, err)

	tc := testCase{
		logger: zaptest.NewLogger(t),

		nsObjects: []runtime.Object{nsObject, nsOtherObject},

		request: req,

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)
			require.NoError(t, err)

			reportList := reporter_v1.ReportList{}
			err = json.Unmarshal(body, &reportList)
			require.NoError(t, err)

			require.Equal(t, 1, len(reportList.Items))
			require.Equal(t, "testns", reportList.Items[0].ObjectMeta.Namespace)
			require.Equal(t, "test", reportList.Items[0].ObjectMeta.Name)
		},
	}

	tc.run(t)
}

func TestFilterLabel(t *testing.T) {
	t.Parallel()

	locationObject := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd",
			Namespace: "testns",
			UID:       "some-uid",
		},
		Spec: form_v1.LocationDescriptorSpec{
			ConfigMapName: "cm1",
			Resources: []form_v1.LocationDescriptorResource{
				{
					Name: "old-resource",
					Type: "some-type",
				},
			},
		},
	}

	nsObject := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "testns",
			Labels: map[string]string{
				voyager.ServiceNameLabel: "test",
			},
		},
	}

	stateObject := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       orch_v1.StateResourceKind,
			APIVersion: orch_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test",
			Namespace: "testns",
		},
		Spec: orch_v1.StateSpec{
			ConfigMapName: "cm1",
			Resources: []orch_v1.StateResource{
				orch_v1.StateResource{
					Name: "messages",
					Type: "sns",
				},
				orch_v1.StateResource{
					Name: "events",
					Type: "sqs",
					DependsOn: []orch_v1.StateDependency{
						orch_v1.StateDependency{
							Name: "messages",
							Attributes: map[string]interface{}{
								"MaxReceiveCount": "100",
							},
						},
					},
				},
			},
		},
		Status: orch_v1.StateStatus{
			Conditions: []cond_v1.Condition{
				{
					Status: cond_v1.ConditionTrue,
					Type:   cond_v1.ConditionInProgress,
				},
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/apis/%s/%s/namespaces/%s/reports?layer=formation",
		reporter.GroupName, currentAPIVersion, "testns",
	), nil)

	require.NoError(t, err)

	tc := testCase{
		logger: zaptest.NewLogger(t),

		formationObjects:    []runtime.Object{locationObject},
		orchestraionObjects: []runtime.Object{stateObject},
		nsObjects:           []runtime.Object{nsObject},

		request: req,

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)
			require.NoError(t, err)

			reportList := reporter_v1.ReportList{}
			err = json.Unmarshal(body, &reportList)
			require.NoError(t, err)

			require.Len(t, reportList.Items, 1)
			require.Equal(t, "testns", reportList.Items[0].ObjectMeta.Namespace)
			require.Equal(t, "test", reportList.Items[0].ObjectMeta.Name)
			require.Len(t, reportList.Items[0].Report.Formation.Resources, 1)
			require.Equal(t, "old-resource", reportList.Items[0].Report.Formation.Resources[0].Name)
			require.Empty(t, reportList.Items[0].Report.Orchestration.Resources)
		},
	}

	tc.run(t)
}

func TestPrometheusInstrumentation(t *testing.T) {
	t.Parallel()

	path := fmt.Sprintf("/apis/%s/%s/namespaces/%s/reports",
		reporter.GroupName, currentAPIVersion, "testns",
	)
	req, err := http.NewRequest(http.MethodGet, path+"?layer=formation", nil)

	require.NoError(t, err)

	tc := testCase{
		logger: zaptest.NewLogger(t),

		formationObjects:    []runtime.Object{},
		orchestraionObjects: []runtime.Object{},
		nsObjects:           []runtime.Object{},

		request: req,

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			var outputMetric io_prometheus_client.Metric
			api.requestDuration.WithLabelValues("200", "GET", "", "testns", path).(prometheus.Histogram).Write(&outputMetric)
			api.requestCounter.WithLabelValues("200", "GET", "", "testns", path).Write(&outputMetric)

			require.Equal(t, uint64(0), *outputMetric.Histogram.Bucket[0].CumulativeCount)
			require.Equal(t, float64(0), *outputMetric.Histogram.Bucket[0].UpperBound)

			require.Equal(t, uint64(1), *outputMetric.Histogram.Bucket[1].CumulativeCount)
			require.Equal(t, float64(0.1), *outputMetric.Histogram.Bucket[1].UpperBound)

			require.Equal(t, float64(1), *outputMetric.Counter.Value)
		},
	}

	tc.run(t)
}

type testCase struct {
	logger *zap.Logger

	compositionObjects  []runtime.Object
	formationObjects    []runtime.Object
	orchestraionObjects []runtime.Object
	executionObjects    []runtime.Object
	scObjects           []runtime.Object
	nsObjects           []runtime.Object

	request *http.Request

	test func(*testing.T, *httptest.ResponseRecorder, *API)
}

func (tc *testCase) run(t *testing.T) {
	compositionClient := compclient_fake.NewSimpleClientset(tc.compositionObjects...)
	formationClient := formclient_fake.NewSimpleClientset(tc.formationObjects...)
	stateClient := orchclient_fake.NewSimpleClientset(tc.orchestraionObjects...)
	smithClient := smithclient_fake.NewSimpleClientset(tc.executionObjects...)
	scClient := scclient_fake.NewSimpleClientset(tc.scObjects...)

	nsClient := coreclient_fake.NewSimpleClientset(tc.nsObjects...)

	testInformers := map[schema.GroupVersionKind]cache.SharedIndexInformer{
		comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind):  compInf.ServiceDescriptorInformer(compositionClient, 0),
		form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind): formInf.LocationDescriptorInformer(formationClient, "", 0),
		orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind):              orchInf.StateInformer(stateClient, "", 0),
		smith_v1.SchemeGroupVersion.WithKind(smith_v1.BundleResourceKind):           bundleClient.BundleInformer(smithClient, "", 0),
		sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"):                      sc_v1b1inf.NewServiceBindingInformer(scClient, "", 0, cache.Indexers{}),
		core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind): core_v1inf.NewNamespaceInformer(nsClient, 0, cache.Indexers{
			ByServiceNameLabelIndexName: ByServiceNameLabelIndex,
		}),
	}

	for _, inf := range testInformers {
		err := inf.AddIndexers(cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})

		require.NoError(t, err)
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	// Start all informers then wait on them
	for _, inf := range testInformers {
		stage.StartWithChannel(inf.Run)
	}
	for _, inf := range testInformers {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	handler := chi.NewRouter()
	api, err := NewReportingAPI(tc.logger, handler, testInformers, &MockASAPConfig{}, voyager.Location{
		EnvType: "dev",
		Account: "account-id",
		Region:  "ap-northsouth-2",
	}, "pkg/reporter/schema/reporter.json", prometheus.NewPedanticRegistry())

	require.NoError(t, err)

	recorder := httptest.NewRecorder()

	reqContext := logz.CreateContextWithLogger(context.Background(), tc.logger)

	reqContext = request.WithUser(reqContext, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
	})
	req := tc.request.WithContext(reqContext)

	req.Header.Add("X-Remote-User", "user")
	req.Header.Add("X-Remote-Group", "groupA")
	req.Header.Add("X-Remote-Group", "groupB")

	handler.ServeHTTP(recorder, req)

	tc.test(t, recorder, api)
}
