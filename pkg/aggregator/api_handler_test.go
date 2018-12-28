package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SermoDigital/jose/jws"
	"github.com/ash2k/stager"
	"github.com/atlassian/voyager"
	agg_v1 "github.com/atlassian/voyager/pkg/apis/aggregator/v1"
	. "github.com/atlassian/voyager/pkg/util/httptest"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/go-chi/chi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	crclient_fake "k8s.io/cluster-registry/pkg/client/clientset/versioned/fake"
	informers "k8s.io/cluster-registry/pkg/client/informers/externalversions"
)

type MockASAPConfig struct{}

func (*MockASAPConfig) GenerateToken(audience string, subject string) ([]byte, error) {
	return []byte("ASAP Token"), nil
}

func (*MockASAPConfig) GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error) {
	return []byte("ASAP Token"), nil
}

func (*MockASAPConfig) KeyID() string     { return "" }
func (*MockASAPConfig) KeyIssuer() string { return "" }

func TestAggregate(t *testing.T) {
	t.Parallel()

	tc := testCase{
		logger: zaptest.NewLogger(t),

		uri: "/test",

		clusters: []mockCluster{
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "dev",
				},
				response: map[string]interface{}{
					"test": "case",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "prod",
				},
				response: map[string]interface{}{
					"test": "prod",
				},
			},
		},

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)

			aggregateList := agg_v1.AggregateList{}
			err = json.Unmarshal(body, &aggregateList)
			require.NoError(t, err)

			expected := agg_v1.AggregateList{
				Items: []agg_v1.Aggregate{
					agg_v1.Aggregate{
						Location: voyager.Location{
							Region:  voyager.Region("ap-southeast-2"),
							EnvType: voyager.EnvType("dev"),
						},
						Name:       "dev.ap-southeast-2",
						StatusCode: 200,
						Body: map[string]interface{}{
							"test": "case",
						},
					},
					agg_v1.Aggregate{
						Location: voyager.Location{
							Region:  voyager.Region("ap-southeast-2"),
							EnvType: voyager.EnvType("prod"),
						},
						Name:       "prod.ap-southeast-2",
						StatusCode: 200,
						Body: map[string]interface{}{
							"test": "prod",
						},
					},
				},
			}

			require.ElementsMatch(t, expected.Items, aggregateList.Items)
		},
	}

	tc.run(t)
}

func TestFilter(t *testing.T) {
	t.Parallel()

	tc := testCase{
		logger: zaptest.NewLogger(t),

		uri:     "/test",
		region:  "ap-southeast-2",
		envType: "dev",

		clusters: []mockCluster{
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "dev",
				},
				response: map[string]interface{}{
					"test": "case",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "prod",
				},
				response: map[string]interface{}{
					"test": "prod",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-northwest-2",
					EnvType: "dev",
				},
				response: map[string]interface{}{
					"test": "region",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "playground",
				},
				response: map[string]interface{}{
					"test": "region",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "integration",
				},
				response: map[string]interface{}{
					"test": "region",
				},
			},
		},

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			body, err := ioutil.ReadAll(recorder.Result().Body)

			aggregateList := agg_v1.AggregateList{}
			err = json.Unmarshal(body, &aggregateList)
			require.NoError(t, err)

			expected := agg_v1.AggregateList{
				Items: []agg_v1.Aggregate{
					agg_v1.Aggregate{
						Location: voyager.Location{
							Region:  voyager.Region("ap-southeast-2"),
							EnvType: voyager.EnvType("dev"),
						},
						Name:       "dev.ap-southeast-2",
						StatusCode: 200,
						Body: map[string]interface{}{
							"test": "case",
						},
					},
				},
			}

			require.ElementsMatch(t, expected.Items, aggregateList.Items)
		},
	}

	tc.run(t)
}

func TestPrometheusInstrumentation(t *testing.T) {
	t.Parallel()

	tc := testCase{
		logger: zaptest.NewLogger(t),

		uri:    "/test",
		region: "ap-southeast-2",

		clusters: []mockCluster{
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "dev",
				},
				response: map[string]interface{}{
					"test": "case",
				},
			},
			mockCluster{
				location: voyager.Location{
					Region:  "ap-southeast-2",
					EnvType: "prod",
				},
				response: map[string]interface{}{
					"test": "prod",
				},
			},
		},

		test: func(t *testing.T, recorder *httptest.ResponseRecorder, api *API) {

			require.Equal(t, http.StatusOK, recorder.Code)

			var outputMetric io_prometheus_client.Metric
			api.requestDuration.WithLabelValues("200", "GET", "ap-southeast-2", "", "/apis/aggregator.voyager.atl-paas.net/v1/aggregate").(prometheus.Histogram).Write(&outputMetric)
			api.requestCounter.WithLabelValues("200", "GET", "ap-southeast-2", "", "/apis/aggregator.voyager.atl-paas.net/v1/aggregate").Write(&outputMetric)

			require.Equal(t, uint64(0), *outputMetric.Histogram.Bucket[0].CumulativeCount)
			require.Equal(t, float64(0), *outputMetric.Histogram.Bucket[0].UpperBound)

			require.Equal(t, uint64(1), *outputMetric.Histogram.Bucket[1].CumulativeCount)
			require.Equal(t, float64(0.1), *outputMetric.Histogram.Bucket[1].UpperBound)

			require.Equal(t, float64(1), *outputMetric.Counter.Value)
		},
	}

	tc.run(t)
}

type mockCluster struct {
	name     string
	location voyager.Location
	response map[string]interface{}
}

type testCase struct {
	logger *zap.Logger

	clusters []mockCluster
	request  *http.Request

	uri     string
	region  string
	envType string

	test func(*testing.T, *httptest.ResponseRecorder, *API)
}

func (tc *testCase) run(t *testing.T) {

	var clusterObjs []runtime.Object
	var servers []*httptest.Server

	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	for _, c := range tc.clusters {
		handler := createMockServer(t, tc.uri, c.response)
		server := httptest.NewServer(handler)

		cluster := &cr_v1a1.Cluster{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: fmt.Sprintf("%s.%s", c.location.EnvType, c.location.Region),
				Labels: map[string]string{
					"region":   string(c.location.Region),
					"paas-env": string(c.location.EnvType),
					"customer": "paas",
				},
			},
			Spec: cr_v1a1.ClusterSpec{
				KubernetesAPIEndpoints: cr_v1a1.KubernetesAPIEndpoints{
					ServerEndpoints: []cr_v1a1.ServerAddressByClientCIDR{
						cr_v1a1.ServerAddressByClientCIDR{
							ServerAddress: server.URL,
						},
					},
				},
			},
		}

		clusterObjs = append(clusterObjs, cluster)
	}

	crclient := crclient_fake.NewSimpleClientset(clusterObjs...)

	clusterInformer := informers.NewSharedInformerFactory(crclient, 0).Clusterregistry().V1alpha1().Clusters().Informer()

	err := clusterInformer.AddIndexers(cache.Indexers{
		ByClusterLabelIndexName: ByClusterLabelIndex,
	})
	require.NoError(t, err)

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	stage.StartWithChannel(clusterInformer.Run)
	require.True(t, cache.WaitForCacheSync(ctx.Done(), clusterInformer.HasSynced))

	handler := chi.NewRouter()
	api, err := NewAPI(tc.logger, handler, clusterInformer, &MockASAPConfig{}, voyager.Location{
		EnvType: "dev",
		Account: "account-id",
		Region:  "ap-northsouth-2",
	}, "pkg/aggregator/schema/aggregator.json", prometheus.NewPedanticRegistry(), []string{"dev", "staging", "prod"})

	require.NoError(t, err)

	recorder := httptest.NewRecorder()

	reqContext := logz.CreateContextWithLogger(context.Background(), tc.logger)

	reqContext = request.WithUser(reqContext, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
	})

	req, err := http.NewRequest("GET", "/apis/aggregator.voyager.atl-paas.net/v1/aggregate", nil)
	require.NoError(t, err)

	q := req.URL.Query()
	q.Add("uri", tc.uri)
	if tc.region != "" {
		q.Add("region", tc.region)
	}
	if tc.envType != "" {
		q.Add("envType", tc.envType)
	}

	req.URL.RawQuery = q.Encode()
	req = req.WithContext(reqContext)
	handler.ServeHTTP(recorder, req)

	tc.test(t, recorder, api)
}

func createMockServer(t *testing.T, uri string, response map[string]interface{}) *HTTPMock {
	blob, err := json.Marshal(response)
	require.NoError(t, err)

	return MockHandler(
		Match(Get, Path(uri)).Respond(
			Status(http.StatusOK),
			BytesBody(blob),
		),
	)
}
