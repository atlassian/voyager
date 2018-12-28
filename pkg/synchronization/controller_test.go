package synchronization

import (
	"context"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	comp_fake "github.com/atlassian/voyager/pkg/composition/client/fake"
	"github.com/atlassian/voyager/pkg/k8s"
	k8s_testing "github.com/atlassian/voyager/pkg/k8s/testing"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/ssam"
	"github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	prom_dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	rbac_v1inf "k8s.io/client-go/informers/rbac/v1"
	k8s_fake "k8s.io/client-go/kubernetes/fake"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

const (
	fakeSCPollErrorCounter = "fake_poll_error_counter"
	fakeUpdateErrorCounter = "fake_access_error_counter"
)

type fakeServiceCentral struct {
	mock.Mock
}

func (m *fakeServiceCentral) GetService(ctx context.Context, user auth.OptionalUser, name string) (*creator_v1.Service, error) {
	args := m.Called(ctx, user, name)
	return args.Get(0).(*creator_v1.Service), args.Error(1)
}

func (m *fakeServiceCentral) ListServices(ctx context.Context, user auth.OptionalUser) ([]creator_v1.Service, error) {
	args := m.Called(ctx, user)
	return args.Get(0).([]creator_v1.Service), args.Error(1)
}

func (m *fakeServiceCentral) ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]creator_v1.Service, error) {
	args := m.Called(ctx, user, modifiedSince)
	return args.Get(0).([]creator_v1.Service), args.Error(1)
}

type fakeReleaseManagement struct {
	serviceName  string
	serviceNames []string
	error        error
	calledParams releases.ResolveParams
	batchParams  releases.ResolveBatchParams
}

func (m *fakeReleaseManagement) Resolve(params releases.ResolveParams) (*releases.ResolvedRelease, error) {
	m.calledParams = params
	if m.error != nil {
		return &releases.ResolvedRelease{}, m.error
	}
	res := defaultReleaseResolveResponse(m.serviceName)
	return &res, nil
}

func (m *fakeReleaseManagement) ResolveLatest(params releases.ResolveBatchParams) ([]releases.ResolvedRelease, time.Time, error) {
	m.batchParams = params
	if m.error != nil {
		return []releases.ResolvedRelease{}, params.From, m.error
	}
	var results []releases.ResolvedRelease
	for _, svc := range m.serviceNames {
		results = append(results, defaultReleaseResolveResponse(svc))
	}
	return results, params.From, nil
}

func TestSkipsConfigMapWhenNotServiceNamespace(t *testing.T) {
	t.Parallel()

	const namespaceName = "the-namespace"
	tc := testCase{
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: namespaceName,
				// no labels means no service
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.mainFake.Actions()
			_, createsFound := findCreatedConfigMap(actions, namespaceName, apisynchronization.DefaultServiceMetadataConfigMapName)
			assert.False(t, createsFound)
			_, updatesFound := findUpdatedConfigMap(actions, namespaceName, apisynchronization.DefaultServiceMetadataConfigMapName)
			assert.False(t, updatesFound)
			_, relCreatesFound := findCreatedConfigMap(actions, namespaceName, releases.DefaultReleaseMetadataConfigMapName)
			assert.False(t, relCreatesFound)
			_, relUpdatesFound := findUpdatedConfigMap(actions, namespaceName, releases.DefaultReleaseMetadataConfigMapName)
			assert.False(t, relUpdatesFound)
		},
	}

	tc.run(t)
}

func TestCreatesConfigMapFromServiceCentralData(t *testing.T) {
	t.Parallel()
	const serviceName = "some-service"
	const namespaceName = "the-namespace"

	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	tc := testCase{
		serviceName:       serviceName,
		ns:                ns,
		mainClientObjects: []runtime.Object{ns, existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			expected := basicServiceProperties(service, voyager.EnvTypeDev)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			actions := tc.mainFake.Actions()

			// Verifying service metadata config map has been successfully created
			cm, _ := findCreatedConfigMap(actions, namespaceName, apisynchronization.DefaultServiceMetadataConfigMapName)
			require.NotNil(t, cm)

			assert.Equal(t, cm.Name, apisynchronization.DefaultServiceMetadataConfigMapName)

			assert.Contains(t, cm.Data, orch_meta.ConfigMapConfigKey)
			data := cm.Data[orch_meta.ConfigMapConfigKey]

			var actual orch_meta.ServiceProperties
			err = yaml.Unmarshal([]byte(data), &actual)
			require.NoError(t, err)

			assert.Equal(t, expected, actual)

			// Verifying releases config map has been successfully created
			relCM, _ := findCreatedConfigMap(actions, namespaceName, releases.DefaultReleaseMetadataConfigMapName)
			require.NotNil(t, relCM)

			assert.Equal(t, relCM.Name, releases.DefaultReleaseMetadataConfigMapName)

			assert.Contains(t, relCM.Data, releases.DataKey)
			relData := relCM.Data[releases.DataKey]

			var actualRelResponse releases.ResolvedReleaseData
			err = yaml.Unmarshal([]byte(relData), &actualRelResponse)
			require.NoError(t, err)

			assert.Equal(t, defaultReleaseResolveResponse(serviceName).ResolvedData, actualRelResponse)
			assert.Equal(t, resolveParams(tc.clusterLocation, serviceName), tc.releasesFake.calledParams)
		},
	}

	tc.run(t)
}

func TestIncludesPagerDutyForClusterEnvironment(t *testing.T) {
	t.Parallel()

	fullPagerDutyMetadata := fullPagerDutyMetadata()

	envTypeCases := []struct {
		envType        voyager.EnvType
		sourceMetadata creator_v1.PagerDutyEnvMetadata
	}{
		{
			voyager.EnvTypeStaging,
			fullPagerDutyMetadata.Staging,
		},
		{
			voyager.EnvTypeProduction,
			fullPagerDutyMetadata.Production,
		},
	}
	const serviceName = "some-service"
	const namespaceName = "the-namespace"

	for _, subCase := range envTypeCases {
		t.Run(string(subCase.envType), func(t *testing.T) {

			ns := &core_v1.Namespace{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.NamespaceKind,
					APIVersion: core_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: namespaceName,
					Labels: map[string]string{
						voyager.ServiceNameLabel: serviceName,
					},
				},
			}

			tc := testCase{
				ns:                ns,
				mainClientObjects: []runtime.Object{ns, existingDefaultDockerSecret()},
				test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
					service := &creator_v1.Service{
						ObjectMeta: meta_v1.ObjectMeta{
							Name: serviceName,
						},
						Spec: creator_v1.ServiceSpec{
							ResourceOwner: "somebody",
							BusinessUnit:  "the unit",
							Metadata: creator_v1.ServiceMetadata{
								PagerDuty: fullPagerDutyMetadata,
							},
						},
					}
					expected := basicServiceProperties(service, subCase.envType)
					cwURL, err := pagerduty.KeyToCloudWatchURL(subCase.sourceMetadata.Main.Integrations.CloudWatch.IntegrationKey)
					require.NoError(t, err)
					expected.Notifications.PagerdutyEndpoint = orch_meta.PagerDuty{
						Generic:    subCase.sourceMetadata.Main.Integrations.Generic.IntegrationKey,
						CloudWatch: cwURL,
					}
					cwURL, err = pagerduty.KeyToCloudWatchURL(subCase.sourceMetadata.LowPriority.Integrations.CloudWatch.IntegrationKey)
					require.NoError(t, err)
					expected.Notifications.LowPriorityPagerdutyEndpoint = orch_meta.PagerDuty{
						Generic:    subCase.sourceMetadata.LowPriority.Integrations.Generic.IntegrationKey,
						CloudWatch: cwURL,
					}

					tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

					// make sure the controller knows we are our specific environment type
					cntrlr.ClusterLocation = voyager.ClusterLocation{
						EnvType: subCase.envType,
					}
					_, err = cntrlr.Process(ctx)
					require.NoError(t, err)

					actions := tc.mainFake.Actions()

					cm, _ := findCreatedConfigMap(actions, namespaceName, apisynchronization.DefaultServiceMetadataConfigMapName)
					require.NotNil(t, cm)

					assert.Equal(t, cm.Name, apisynchronization.DefaultServiceMetadataConfigMapName)

					assert.Contains(t, cm.Data, orch_meta.ConfigMapConfigKey)
					data := cm.Data[orch_meta.ConfigMapConfigKey]

					var actual orch_meta.ServiceProperties
					err = yaml.Unmarshal([]byte(data), &actual)
					require.NoError(t, err)

					assert.Equal(t, expected, actual)
				},
			}
			tc.run(t)
		})
	}
}

func TestReturnsErrorWhenPagerDutyNotPresent(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret(), ns},
		ns:                ns,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					Metadata:      creator_v1.ServiceMetadata{
						// no pagerduty metadata
					},
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			// we have not set pagerduty for this environment
			cntrlr.ClusterLocation = voyager.ClusterLocation{
				EnvType: voyager.EnvTypeProduction,
			}
			retriable, err := cntrlr.Process(ctx)
			require.Error(t, err)

			assert.False(t, retriable)
		},
	}

	tc.run(t)
}

func TestReturnsErrorWhenPagerDutyEmptyForEnvironment(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingDefaultDockerSecret(), ns},
		ns:                ns,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{}, // empty object
					},
				},
			}

			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			// we have not set pagerduty for this environment
			cntrlr.ClusterLocation = voyager.ClusterLocation{
				EnvType: voyager.EnvTypeProduction,
			}
			retriable, err := cntrlr.Process(ctx)
			require.Error(t, err)

			assert.False(t, retriable)
		},
	}

	tc.run(t)
}

func TestUpdatesExistingConfigMap(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	oldData, err := yaml.Marshal(orch_meta.ServiceProperties{
		ResourceOwner: "old owner",
		BusinessUnit:  "old unit",
	})
	oldRelData, err := yaml.Marshal(releases.ResolvedRelease{
		ServiceName:  "svc",
		Label:        "",
		ResolvedData: map[string]map[string]interface{}{},
	})
	require.NoError(t, err)
	existingNS := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}
	existingCM := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            apisynchronization.DefaultServiceMetadataConfigMapName,
			Namespace:       existingNS.GetName(),
			UID:             "some-uid",
			ResourceVersion: "some-resource-version",
		},
		Data: map[string]string{
			orch_meta.ConfigMapConfigKey: string(oldData),
		},
	}
	existingRelCM := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            releases.DefaultReleaseMetadataConfigMapName,
			Namespace:       existingNS.GetName(),
			UID:             "some-rel-uid",
			ResourceVersion: "some-resource-rel-version",
		},
		Data: map[string]string{
			releases.DataKey: string(oldRelData),
		},
	}

	tc := testCase{
		serviceName:       serviceName,
		mainClientObjects: []runtime.Object{existingNS, existingCM, existingRelCM, existingDefaultDockerSecret()},
		ns:                existingNS,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "new unit",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
				},
			}

			expected := basicServiceProperties(service, voyager.EnvTypeDev)
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			actions := tc.mainFake.Actions()

			// Verifying service metadata config map has been updated
			cm, _ := findUpdatedConfigMap(actions, existingNS.GetName(), apisynchronization.DefaultServiceMetadataConfigMapName)
			require.NotNil(t, cm)

			require.Equal(t, cm.Name, apisynchronization.DefaultServiceMetadataConfigMapName)
			assert.Equal(t, existingCM.GetUID(), cm.GetUID())
			assert.Equal(t, existingCM.GetResourceVersion(), cm.GetResourceVersion())

			assert.Contains(t, cm.Data, orch_meta.ConfigMapConfigKey)
			data := cm.Data[orch_meta.ConfigMapConfigKey]

			var actual orch_meta.ServiceProperties
			err = yaml.Unmarshal([]byte(data), &actual)
			require.NoError(t, err)

			assert.Equal(t, expected, actual)

			// Verifying releases config map has been updated
			relCM, _ := findUpdatedConfigMap(actions, existingNS.GetName(), releases.DefaultReleaseMetadataConfigMapName)
			require.NotNil(t, relCM)

			require.Equal(t, relCM.Name, releases.DefaultReleaseMetadataConfigMapName)
			assert.Equal(t, existingRelCM.GetUID(), relCM.GetUID())
			assert.Equal(t, existingRelCM.GetResourceVersion(), relCM.GetResourceVersion())

			assert.Contains(t, relCM.Data, releases.DataKey)
			data = relCM.Data[releases.DataKey]

			var actualRelResponse releases.ResolvedReleaseData
			err = yaml.Unmarshal([]byte(data), &actualRelResponse)
			require.NoError(t, err)

			assert.Equal(t, defaultReleaseResolveResponse(serviceName).ResolvedData, actualRelResponse)
			assert.Equal(t, resolveParams(tc.clusterLocation, serviceName), tc.releasesFake.calledParams)
		},
	}

	tc.run(t)
}

func TestSkipsConfigMapUpdateWhenMetadataIsTheSame(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	existingService := &creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceName,
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: "somebody",
			BusinessUnit:  "some unit",
			Metadata: creator_v1.ServiceMetadata{
				PagerDuty: &creator_v1.PagerDutyMetadata{},
			},
		},
	}

	oldData, err := yaml.Marshal(basicServiceProperties(existingService, voyager.EnvTypeDev))
	require.NoError(t, err)
	oldRelData, err := yaml.Marshal(defaultReleaseResolveResponse(serviceName).ResolvedData)
	require.NoError(t, err)
	existingNS := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}
	existingCM := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            apisynchronization.DefaultServiceMetadataConfigMapName,
			Namespace:       existingNS.GetName(),
			UID:             "some-uid",
			ResourceVersion: "some-resource-version",
		},
		Data: map[string]string{
			orch_meta.ConfigMapConfigKey: string(oldData),
		},
	}
	existingRelCM := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            releases.DefaultReleaseMetadataConfigMapName,
			Namespace:       existingNS.GetName(),
			UID:             "some-rel-uid",
			ResourceVersion: "some-rel-resource-version",
		},
		Data: map[string]string{
			releases.DataKey: string(oldRelData),
		},
	}

	tc := testCase{
		serviceName:       serviceName,
		mainClientObjects: []runtime.Object{existingNS, existingCM, existingRelCM, existingDefaultDockerSecret()},
		ns:                existingNS,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(existingService, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			actions := tc.mainFake.Actions()

			_, hasUpdate := findUpdatedConfigMap(actions, existingNS.GetName(), apisynchronization.DefaultServiceMetadataConfigMapName)
			assert.False(t, hasUpdate)

			_, hasUpdate = findUpdatedConfigMap(actions, existingNS.GetName(), releases.DefaultReleaseMetadataConfigMapName)
			assert.False(t, hasUpdate)
		},
	}

	tc.run(t)
}

func TestMarksRetriableWhenNotKnownService(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	existingNS := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}
	existingCM := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            apisynchronization.DefaultServiceMetadataConfigMapName,
			Namespace:       existingNS.GetName(),
			UID:             "some-uid",
			ResourceVersion: "some-resource-version",
		},
		Data: map[string]string{
			orch_meta.ConfigMapConfigKey: "",
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingNS, existingCM, existingDefaultDockerSecret()},
		ns:                existingNS,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(&creator_v1.Service{}, errors.Errorf("Could not find service"))

			retriable, err := cntrlr.Process(ctx)

			require.Error(t, err)
			assert.True(t, retriable)
		},
	}

	tc.run(t)
}

func TestFetchesAllServicesFromServiceCentral(t *testing.T) {
	t.Parallel()

	tc := testCase{
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{}, nil)

			cntrlr.syncServiceMetadata()
		},
	}

	tc.run(t)
}

func TestFetchesModifiedServicesFromServiceCentral(t *testing.T) {
	t.Parallel()

	tc := testCase{
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			now := time.Now()
			cntrlr.LastFetchedAllServices = &now // Make the switch to ListModifiedServices
			cntrlr.LastFetchedModifiedServices = &now
			tc.scFake.On("ListModifiedServices", mock.Anything, auth.NoUser(), mock.Anything).Return([]creator_v1.Service{}, nil)

			cntrlr.syncServiceMetadata()
		},
	}

	tc.run(t)
}

func TestFetchesServicesFromServiceCentral(t *testing.T) {
	t.Parallel()

	tc := testCase{
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			// Only Once()
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Once().Return([]creator_v1.Service{}, nil)
			cntrlr.syncServiceMetadata()

			// On the second call it should switch to ListModifiedServices
			tc.scFake.On("ListModifiedServices", mock.Anything, auth.NoUser(), mock.Anything).Once().Return([]creator_v1.Service{}, nil)
			cntrlr.syncServiceMetadata()
		},
	}

	tc.run(t)
}

func TestIncrementsCounterWhenFailsCallingServiceCentral(t *testing.T) {
	t.Parallel()

	tc := testCase{
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			tc.scFake.On("ListServices", mock.Anything, auth.NoUser()).Return([]creator_v1.Service{}, errors.New("failed calling SC or something"))

			cntrlr.syncServiceMetadata()

			ctr, err := tc.findCounter(fakeSCPollErrorCounter)
			require.NoError(t, err)
			require.NotNil(t, ctr)

			assert.Equal(t, float64(1), ctr.GetValue())
		},
	}

	tc.run(t)
}

func TestCreatesAllServicesReleaseData(t *testing.T) {
	t.Parallel()

	service1Name := "service-1"
	service2Name := "service-2"

	service1 := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: service1Name,
			Labels: map[string]string{
				voyager.ServiceNameLabel: service1Name,
			},
		},
	}
	service2 := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: service2Name,
			Labels: map[string]string{
				voyager.ServiceNameLabel: service2Name,
			},
		},
	}
	tc := testCase{
		releaseDataServiceNames: []string{service1Name, service2Name},
		mainClientObjects:       []runtime.Object{service1, service2, existingDefaultDockerSecret()},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			cntrlr.syncReleasesMetadata()

			actions := tc.mainFake.Actions()

			createdConfigMaps := make([]string, 0)
			for _, action := range k8s_testing.FilterCreateActions(actions) {
				if r, ok := action.GetObject().(*core_v1.ConfigMap); ok {
					createdConfigMaps = append(createdConfigMaps, r.Namespace)
				}
			}
			assert.Equal(t, 2, len(createdConfigMaps))
			assert.Contains(t, createdConfigMaps, service1Name, service2Name)
		},
	}

	tc.run(t)
}

func TestCreatesDockerSecret(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{ns, existingDefaultDockerSecret()},
		ns:                ns,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			secrets := findCreatedSecrets(tc.mainFake.Actions())

			// ensure secret was created
			secret, ok := secrets[existingDefaultDockerSecret().Name]
			assert.True(t, ok)

			// check secret namespace and contents
			assert.Equal(t, tc.ns.Name, secret.Namespace, "Should be in the created namespace")
			assert.True(t, reflect.DeepEqual(existingDefaultDockerSecret().Data, secret.Data), "Should contain the same data as the copied secret")

			// check secret UID does not match the copied UID
			assert.NotEqual(t, existingDefaultDockerSecret().UID, secret.UID)

			// check secret owner references
			assert.Nil(t, secret.OwnerReferences, "OwnerReferences should be nil")

			// check resource version
			assert.Equal(t, "", secret.ResourceVersion, "ResourceVersion should be reset")
		},
	}

	tc.run(t)
}

func TestUpdatesDockerSecret(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	existingDockerSecret := &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.SecretKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      dockerSecretName,
			Namespace: existingNamespace.Name,
			UID:       "existing-docker-secret-uid",
		},
		// intentionally incorrect secret type to assert
		// it is fixed during update
		Type: core_v1.SecretTypeOpaque,

		// intentionally incorrect secret data to assert
		// it is fixed during update
		Data: map[string][]byte{},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{existingNamespace, existingDefaultDockerSecret(), existingDockerSecret},
		ns:                existingNamespace,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			secrets := findUpdatedSecrets(tc.mainFake.Actions())

			// Ensure secret exists
			secret, ok := secrets[existingDockerSecret.Name]
			assert.True(t, ok)
			assert.Equal(t, existingDockerSecret.GetUID(), secret.GetUID())

			// Ensure the secret type and data has been updated
			assert.Equal(t, existingDefaultDockerSecret().Type, secret.Type)
			assert.True(t, reflect.DeepEqual(existingDefaultDockerSecret().Data, secret.Data))
		},
	}

	tc.run(t)
}

func TestDockerSecretNonExistent(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	tc := testCase{
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "the-namespace",
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			assert.Error(t, err, "Should return an error as the docker secret does not exist")
		},
	}

	tc.run(t)
}

func TestDockerSecretIncorrectType(t *testing.T) {
	t.Parallel()

	existingSecret := existingDefaultDockerSecret()
	existingSecret.Type = core_v1.SecretTypeOpaque

	const serviceName = "some-service"
	tc := testCase{
		mainClientObjects: []runtime.Object{existingSecret},
		ns: &core_v1.Namespace{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "the-namespace",
				Labels: map[string]string{
					voyager.ServiceNameLabel: serviceName,
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			assert.Error(t, err, "Should return an error as the docker secret is of the wrong type")

		},
	}

	tc.run(t)
}

func TestAddsKube2IamAnnotation(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{ns, existingDefaultDockerSecret()},
		ns:                ns,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			// Ensure the namespace is updated
			updatedNamespaces := findUpdatedNamespaces(tc.mainFake.Actions())
			namespace, ok := updatedNamespaces[ns.Name]
			assert.True(t, ok)

			// Ensure the namespace has the annotation
			val, ok := namespace.Annotations[allowedRolesAnnotation]
			assert.True(t, ok)

			// Ensure the value of the annotation is correct
			expectedVal, err := cntrlr.getNamespaceAllowedRoles(serviceName)
			require.NoError(t, err)
			assert.Equal(t, expectedVal, val)
		},
	}

	tc.run(t)
}

func TestUpdatesKube2IamAnnotation(t *testing.T) {
	t.Parallel()

	const serviceName = "some-service"
	ns := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "the-namespace",
			Labels: map[string]string{
				voyager.ServiceNameLabel: serviceName,
			},
			Annotations: map[string]string{
				allowedRolesAnnotation: "incorrect-annotation",
			},
		},
	}

	tc := testCase{
		mainClientObjects: []runtime.Object{ns, existingDefaultDockerSecret()},
		ns:                ns,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			service := &creator_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: creator_v1.ServiceSpec{
					ResourceOwner: "somebody",
					BusinessUnit:  "the unit",
					LoggingID:     "some-logging-id",
					Metadata: creator_v1.ServiceMetadata{
						PagerDuty: &creator_v1.PagerDutyMetadata{},
					},
					SSAMContainerName: "some-ssam-container",
					ResourceTags: map[voyager.Tag]string{
						"foo": "bar",
						"baz": "blah",
					},
				},
			}
			tc.scFake.On("GetService", mock.Anything, auth.NoUser(), serviceName).Return(service, nil)

			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			// Ensure the namespace is updated
			updatedNamespaces := findUpdatedNamespaces(tc.mainFake.Actions())
			namespace, ok := updatedNamespaces[ns.Name]
			assert.True(t, ok)

			// Ensure the namespace has the annotation
			val, ok := namespace.Annotations[allowedRolesAnnotation]
			assert.True(t, ok)

			// Ensure the value of the annotation is correct
			expectedVal, err := cntrlr.getNamespaceAllowedRoles(serviceName)
			require.NoError(t, err)
			assert.Equal(t, expectedVal, val)
		},
	}

	tc.run(t)
}

func TestGenerateIamRoleGlob(t *testing.T) {
	t.Parallel()

	cases := []struct {
		account     voyager.Account
		serviceName string
		want        string
	}{
		{
			account:     voyager.Account("12345"),
			serviceName: "test-svc",
			want:        "arn:aws:iam::12345:role/rps-test-svc-*",
		},
	}
	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, c.want, generateIamRoleGlob(c.account, c.serviceName))
		})
	}
}

func basicServiceProperties(s *creator_v1.Service, envType voyager.EnvType) orch_meta.ServiceProperties {
	return orch_meta.ServiceProperties{
		ResourceOwner: s.Spec.ResourceOwner,
		BusinessUnit:  s.Spec.BusinessUnit,
		Notifications: orch_meta.Notifications{
			Email: s.Spec.EmailAddress(),
			LowPriorityPagerdutyEndpoint: orch_meta.PagerDuty{
				Generic:    "5d11612f25b840faaf77422edeff9c76",
				CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/124e0f010f214a9b9f30b768e7b18e69",
			},
			PagerdutyEndpoint: orch_meta.PagerDuty{
				Generic:    "5d11612f25b840faaf77422edeff9c76",
				CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/124e0f010f214a9b9f30b768e7b18e69",
			},
		},
		LoggingID:       s.Spec.LoggingID,
		SSAMAccessLevel: ssam.AccessLevelNameForEnvType(s.Spec.SSAMContainerName, envType),
		UserTags:        s.Spec.ResourceTags,
	}
}

type testCase struct {
	mainClientObjects []runtime.Object
	compClientObjects []runtime.Object

	clusterLocation voyager.ClusterLocation
	ns              *core_v1.Namespace

	mainFake     *kube_testing.Fake
	compFake     *kube_testing.Fake
	scFake       *fakeServiceCentral
	releasesFake *fakeReleaseManagement
	registry     *prometheus.Registry
	serviceName  string
	// Each service name listed will have some fake release metadata made available for it
	releaseDataServiceNames []string

	test func(*testing.T, *Controller, *ctrl.ProcessContext, *testCase)
}

func (tc *testCase) run(t *testing.T) {
	mainClient := k8s_fake.NewSimpleClientset(tc.mainClientObjects...)
	compClient := comp_fake.NewSimpleClientset(tc.compClientObjects...)
	tc.clusterLocation = voyager.ClusterLocation{
		Account: "123", Region: "us-west-1", EnvType: "dev",
	}
	tc.mainFake = &mainClient.Fake
	tc.compFake = &compClient.Fake

	tc.scFake = new(fakeServiceCentral)
	tc.releasesFake = &fakeReleaseManagement{serviceName: tc.serviceName, serviceNames: tc.releaseDataServiceNames}

	tc.registry = prometheus.NewRegistry()

	ctrlr, pctx, close := tc.newController(t, mainClient, compClient)
	defer close()

	pctx.Object = tc.ns

	tc.test(t, ctrlr, pctx, tc)
}

func (tc *testCase) findCounter(name string) (*prom_dto.Counter, error) {
	mfs, err := tc.registry.Gather()
	if err != nil {
		return nil, err
	}

	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf.GetMetric()[0].GetCounter(), nil
		}
	}

	return nil, nil
}

func (tc *testCase) newController(t *testing.T, mainClient *k8s_fake.Clientset, compClient *comp_fake.Clientset) (*Controller, *ctrl.ProcessContext, func()) {
	logger := zaptest.NewLogger(t)
	config := &ctrl.Config{
		Logger:       logger,
		ResyncPeriod: time.Second * 60,
		MainClient:   mainClient,
	}

	namespaceInformer := core_v1inf.NewNamespaceInformer(mainClient, config.ResyncPeriod, cache.Indexers{
		NamespaceByServiceLabelIndexName: NsServiceLabelIndexFunc,
	})
	configMapInformer := core_v1inf.NewConfigMapInformer(mainClient, meta_v1.NamespaceAll, config.ResyncPeriod, cache.Indexers{})
	roleBindingInformer := rbac_v1inf.NewRoleBindingInformer(mainClient, meta_v1.NamespaceAll, config.ResyncPeriod, cache.Indexers{})
	crInf := rbac_v1inf.NewClusterRoleInformer(mainClient, config.ResyncPeriod, cache.Indexers{})
	crbInf := rbac_v1inf.NewClusterRoleBindingInformer(mainClient, config.ResyncPeriod, cache.Indexers{})

	informers := []cache.SharedIndexInformer{namespaceInformer, configMapInformer, roleBindingInformer, crInf, crbInf}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

	stgr := stager.New()
	stage := stgr.NextStage()

	// Start all informers then wait on them
	for _, inf := range informers {
		stage.StartWithChannel(inf.Run)
	}
	for _, inf := range informers {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	// setup fake metrics
	scec := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: fakeSCPollErrorCounter,
			Help: "some help string",
		})
	auec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: fakeUpdateErrorCounter,
			Help: "some help string",
		},
		[]string{"service_name"},
	)
	tc.registry.MustRegister(scec, auec)

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	roleBindingObjectUpdater := updater.RoleBindingUpdater(roleBindingInformer.GetIndexer(), specCheck, config.MainClient)
	configMapObjectUpdater := updater.ConfigMapUpdater(configMapInformer.GetIndexer(), specCheck, config.MainClient)
	namespaceObjectUpdater := updater.NamespaceUpdater(namespaceInformer.GetIndexer(), specCheck, config.MainClient)
	clusterRoleObjectUpdater := updater.ClusterRoleUpdater(crInf.GetIndexer(), specCheck, config.MainClient)
	clusterRoleBindingObjectUpdater := updater.ClusterRoleBindingUpdater(crbInf.GetIndexer(), specCheck, config.MainClient)

	ctrlr := &Controller{
		Logger: logger,

		MainClient: mainClient,
		CompClient: compClient,

		NamespaceInformer: namespaceInformer,
		ConfigMapInformer: configMapInformer,

		ServiceCentral:    tc.scFake,
		ClusterLocation:   tc.clusterLocation,
		ReleaseManagement: tc.releasesFake,

		RoleBindingUpdater:        roleBindingObjectUpdater,
		ConfigMapUpdater:          configMapObjectUpdater,
		NamespaceUpdater:          namespaceObjectUpdater,
		ClusterRoleUpdater:        clusterRoleObjectUpdater,
		ClusterRoleBindingUpdater: clusterRoleBindingObjectUpdater,

		ServiceCentralPollErrorCounter: scec,
		AccessUpdateErrorCounter:       auec,

		AllowMutateServices: true,
	}

	pctx := &ctrl.ProcessContext{
		Logger: logger,
	}

	return ctrlr, pctx, func() {
		stgr.Shutdown()
		cancel()
	}
}

func findCreatedConfigMap(actions []kube_testing.Action, nsName string, name string) (*core_v1.ConfigMap, bool) {
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if r, ok := action.GetObject().(*core_v1.ConfigMap); ok && r.ObjectMeta.Namespace == nsName && r.ObjectMeta.Name == name {
			return r, true
		}
	}
	return nil, false
}

func findUpdatedConfigMap(actions []kube_testing.Action, nsName string, name string) (*core_v1.ConfigMap, bool) {
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if r, ok := action.GetObject().(*core_v1.ConfigMap); ok && r.ObjectMeta.Namespace == nsName && r.ObjectMeta.Name == name {
			return r, true
		}
	}
	return nil, false
}

func findUpdatedNamespaces(actions []kube_testing.Action) map[string]*core_v1.Namespace {
	result := make(map[string]*core_v1.Namespace)
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if namespace, ok := action.GetObject().(*core_v1.Namespace); ok {
			result[namespace.Name] = namespace
		}
	}
	return result
}

func resolveParams(location voyager.ClusterLocation, serviceName string) releases.ResolveParams {
	return releases.ResolveParams{
		Region: location.Region, Account: location.Account, Environment: location.EnvType,
		ServiceName: voyager.ServiceName(serviceName),
	}
}

func findCreatedSecrets(actions []kube_testing.Action) map[string]*core_v1.Secret {
	result := make(map[string]*core_v1.Secret)
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if secret, ok := action.GetObject().(*core_v1.Secret); ok {
			result[secret.Name] = secret
		}
	}
	return result
}

func findUpdatedSecrets(actions []kube_testing.Action) map[string]*core_v1.Secret {
	result := make(map[string]*core_v1.Secret)
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if secret, ok := action.GetObject().(*core_v1.Secret); ok {
			result[secret.Name] = secret
		}
	}
	return result
}

func fullPagerDutyMetadata() *creator_v1.PagerDutyMetadata {
	return &creator_v1.PagerDutyMetadata{
		Production: creator_v1.PagerDutyEnvMetadata{
			Main: creator_v1.PagerDutyServiceMetadata{
				ServiceID: "prod-main-service",
				PolicyID:  "prod-main-policy",
				Integrations: creator_v1.PagerDutyIntegrations{
					CloudWatch: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-prod-main-cloudwatch",
					},
					Generic: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-prod-main-generic",
					},
				},
			},
			LowPriority: creator_v1.PagerDutyServiceMetadata{
				ServiceID: "prod-low-service",
				PolicyID:  "prod-low-policy",
				Integrations: creator_v1.PagerDutyIntegrations{
					CloudWatch: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-prod-low-cloudwatch",
					},
					Generic: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-prod-low-generic",
					},
				},
			},
		},
		Staging: creator_v1.PagerDutyEnvMetadata{
			Main: creator_v1.PagerDutyServiceMetadata{
				ServiceID: "stg-main-service",
				PolicyID:  "stg-main-policy",
				Integrations: creator_v1.PagerDutyIntegrations{
					CloudWatch: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-stg-main-cloudwatch",
					},
					Generic: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-stg-main-generic",
					},
				},
			},
			LowPriority: creator_v1.PagerDutyServiceMetadata{
				ServiceID: "stg-low-service",
				PolicyID:  "stg-low-policy",
				Integrations: creator_v1.PagerDutyIntegrations{
					CloudWatch: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-stg-low-cloudwatch",
					},
					Generic: creator_v1.PagerDutyIntegrationMetadata{
						IntegrationKey: "some-stg-low-generic",
					},
				},
			},
		},
	}
}

func defaultReleaseResolveResponse(serviceName string) releases.ResolvedRelease {
	return releases.ResolvedRelease{
		ServiceName: serviceName,
		Label:       "",
		ResolvedData: map[string]map[string]interface{}{
			"artifact1": {"tag": "0.0.1"},
		},
	}
}

// existingDefaultDockerSecret returns an existing default docker cfg secret in the namespace it is expected to be in
func existingDefaultDockerSecret() *core_v1.Secret {
	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.SecretKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            dockerSecretName,
			Namespace:       dockerSecretNamespace,
			UID:             "default-docker-secret",
			ResourceVersion: "12345",
		},
		Type: core_v1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte("docker-secret"),
		},
	}
}
