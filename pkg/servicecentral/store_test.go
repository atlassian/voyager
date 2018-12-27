package servicecentral

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var testCreationTime, _ = time.Parse(time.RFC3339, testCreationTimestamp)

// Making sure that the actual type implements the interface
var _ serviceCentralClient = &Client{}

type serviceCentralClientMock struct {
	mock.Mock
}

// Making sure that mock implements the interface
var _ serviceCentralClient = &serviceCentralClientMock{}

func (m *serviceCentralClientMock) CreateService(ctx context.Context, user auth.User, data *ServiceData) (*ServiceData, error) {
	args := m.Called(ctx, user, data)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*ServiceData), args.Error(1)
}

func (m *serviceCentralClientMock) ListServices(ctx context.Context, user auth.OptionalUser, search string) ([]ServiceData, error) {
	args := m.Called(ctx, user, search)
	return args.Get(0).([]ServiceData), args.Error(1)
}

func (m *serviceCentralClientMock) ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]ServiceData, error) {
	args := m.Called(ctx, user, modifiedSince)
	return args.Get(0).([]ServiceData), args.Error(1)
}

func (m *serviceCentralClientMock) GetService(ctx context.Context, user auth.OptionalUser, serviceUUID string) (*ServiceData, error) {
	args := m.Called(ctx, user, serviceUUID)
	ret1, _ := args.Get(0).(*ServiceData)
	return ret1, args.Error(1)
}

func (m *serviceCentralClientMock) PatchService(ctx context.Context, user auth.User, data *ServiceData) error {
	args := m.Called(ctx, user, data)
	return args.Error(0)
}

func (m *serviceCentralClientMock) DeleteService(ctx context.Context, user auth.User, serviceUUID string) error {
	args := m.Called(ctx, user, serviceUUID)
	return args.Error(0)
}

func TestCreateService(t *testing.T) {
	t.Parallel()
	// given
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("CreateService", mock.Anything, testUser, newServiceServiceCentralData(false)).Return(newServiceServiceCentralData(true), nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)
	// when
	result, err := store.FindOrCreateService(context.Background(), testUser, newService(false))
	// then
	require.NoError(t, err)
	assert.Equal(t, newService(true), result)
}

func TestCreateServiceSucceedsIfServiceWithTheSameDataExistsStore(t *testing.T) {
	t.Parallel()
	// given
	existingServiceData := newServiceServiceCentralData(true)
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("CreateService", mock.Anything, mock.Anything, mock.Anything).Return((*ServiceData)(nil), httputil.NewConflict("already exists"))
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, "service_name='test-service'").Return([]ServiceData{*existingServiceData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, *existingServiceData.ServiceUUID).Return(existingServiceData, nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)
	// when
	data, err := store.FindOrCreateService(context.Background(), testUser, newService(false))
	// then
	require.NoError(t, err)
	assert.Equal(t, newService(true), data)
}

func TestCreateServiceFailsIfServiceWithTheSameNameButDifferentOwnerExistsStore(t *testing.T) {
	t.Parallel()
	// given
	existingServiceData := newServiceServiceCentralData(true)
	existingServiceData.ServiceOwner.Username = "somebody-else"
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("CreateService", mock.Anything, mock.Anything, mock.Anything).Return((*ServiceData)(nil), httputil.NewConflict("already exists"))
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, "service_name='test-service'").Return([]ServiceData{*existingServiceData}, nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)
	// when
	_, err := store.FindOrCreateService(context.Background(), testUser, newService(false))
	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), testUser.Name()+`" not allowed to use service owned by "somebody-else"`)
}

func TestCreateServiceFailsIfServiceExistsButCouldNotBeFound(t *testing.T) {
	t.Parallel()
	// given
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("CreateService", mock.Anything, mock.Anything, mock.Anything).Return((*ServiceData)(nil), httputil.NewConflict("already exists"))
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, "service_name='test-service'").Return([]ServiceData{}, nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)
	// when
	_, err := store.FindOrCreateService(context.Background(), testUser, newService(false))
	// then
	assert.Error(t, err) // no panic
}

func TestGetServiceSearchesByNameAndPlatform(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, mock.Anything).Return(expectedData, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	_, err := lister.GetService(context.Background(), optionalUser, testServiceName)

	// then
	require.NoError(t, err)
	expectedSearch := fmt.Sprintf("service_name='%s' AND platform='micros2'", testServiceName)
	serviceCentralClient.AssertCalled(t, "ListServices", mock.Anything, mock.Anything, expectedSearch)
}

func TestGetServiceReturnsResult(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedService := newService(true)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, optionalUser, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, optionalUser, mock.Anything).Return(expectedData, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	result, err := lister.GetService(context.Background(), optionalUser, testServiceName)

	// then
	require.NoError(t, err)
	assert.Equal(t, expectedService, result)
}

func TestGetServiceWithComplianceReturnsResult(t *testing.T) {
	t.Parallel()

	// given
	testCases := []struct {
		expectedVal bool
	}{
		{true},
		{false},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("PRGB=%v", tc.expectedVal), func(t *testing.T) {
			serviceCentralClient := new(serviceCentralClientMock)
			expectedService := newServiceWithCompliance(true, tc.expectedVal)
			expectedData := newTestServiceData(true)
			expectedData.Compliance = &ServiceComplianceConf{
				PRGBControl: &tc.expectedVal,
			}

			serviceCentralClient.On("ListServices", mock.Anything, optionalUser, mock.Anything).Return([]ServiceData{*expectedData}, nil)
			serviceCentralClient.On("GetService", mock.Anything, optionalUser, mock.Anything).Return(expectedData, nil)
			lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

			// when
			result, err := lister.GetService(context.Background(), optionalUser, testServiceName)

			// then
			require.NoError(t, err)
			assert.Equal(t, expectedService, result)
		})
	}
}

func TestGetServiceReturnsSingleServiceIfMultiple(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedService := newService(true)
	expectedData := newTestServiceData(true)
	expectedData2 := newTestServiceData(true)
	otherUuid := "some-uuid"
	expectedData2.ServiceUUID = &otherUuid
	expectedData2.ServiceName = "other-name"
	serviceCentralClient.On("ListServices", mock.Anything, optionalUser, mock.Anything).Return([]ServiceData{*expectedData2, *expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, optionalUser, testServiceUUID).Return(expectedData, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	result, err := lister.GetService(context.Background(), optionalUser, testServiceName)

	// then
	require.NoError(t, err)
	assert.Equal(t, expectedService, result)
}

func TestGetServiceIncludesTags(t *testing.T) {
	t.Parallel()

	// given
	const serviceName = "some-service"
	serviceCentralClient := new(serviceCentralClientMock)

	sd := newTestServiceData(true)
	sd.Tags = []string{"will be discarded", "micros2:first=some=value", "micros2:second=space in value"}
	sd.ServiceName = serviceName

	expectedService := newService(true)
	expectedService.Spec.ResourceTags = parsePlatformTags(sd.Tags)
	serviceCentralClient.On("ListServices", mock.Anything, optionalUser, mock.Anything).Return([]ServiceData{*sd}, nil)
	serviceCentralClient.On("GetService", mock.Anything, optionalUser, *sd.ServiceUUID).Return(sd, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	result, err := lister.GetService(context.Background(), optionalUser, serviceName)

	// then
	require.NoError(t, err)
	assert.Equal(t, expectedService.Spec, result.Spec)
}

func TestGetServiceReturnsErrorWhenServiceNotFound(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("ListServices", mock.Anything, optionalUser, mock.Anything).Return([]ServiceData{}, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	_, err := lister.GetService(context.Background(), optionalUser, testServiceName)

	// then
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
}

func TestPatchServiceUpdatesTagsRetainsPreviousValues(t *testing.T) {
	t.Parallel()

	// given
	const serviceName = "some-service"
	serviceCentralClient := new(serviceCentralClientMock)

	updatedService := &creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceName,
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: "some-owner",
			ResourceTags: map[voyager.Tag]string{
				"foo":    "new-value",
				"newtag": "somevalue",
			},
		},
	}

	serviceUUID := "some-uuid"
	oldService := ServiceData{
		ServiceUUID: &serviceUUID,
		ServiceName: serviceName,
		Tags:        []string{"random tag"},
	}
	expectedTags := append(oldService.Tags, convertPlatformTags(updatedService.Spec.ResourceTags)...)

	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{oldService}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, serviceUUID).Return(&oldService, nil)
	serviceCentralClient.On("PatchService", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := store.PatchService(context.Background(), testUser, updatedService)

	// then
	require.NoError(t, err)

	serviceCentralClient.AssertNumberOfCalls(t, "PatchService", 1)
	for _, call := range serviceCentralClient.Calls {
		if call.Method != "PatchService" {
			continue
		}

		data, ok := call.Arguments[2].(*ServiceData)
		require.True(t, ok)

		// we only care about the tags for this test, and order isn't guaranteed
		// so we assert against the elements
		assert.ElementsMatch(t, expectedTags, data.Tags)
	}
}

func TestPatchServiceRetainsMiscValues(t *testing.T) {
	t.Parallel()

	// given
	const serviceName = "some-service"
	serviceCentralClient := new(serviceCentralClientMock)

	updatedService := &creator_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceName,
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: "some-owner",
			Metadata: creator_v1.ServiceMetadata{
				PagerDuty: &creator_v1.PagerDutyMetadata{
					Production: creator_v1.PagerDutyEnvMetadata{
						Main: creator_v1.PagerDutyServiceMetadata{
							ServiceID: "some-service",
						},
					},
				},
				Bamboo: &creator_v1.BambooMetadata{
					Builds: []creator_v1.BambooPlanRef{
						{"server", "plan"},
					},
				},
			},
		},
	}
	pdMeta, err := json.Marshal(updatedService.Spec.Metadata.PagerDuty)
	require.NoError(t, err)
	bambooMeta, err := json.Marshal(updatedService.Spec.Metadata.Bamboo)
	require.NoError(t, err)

	serviceUUID := "some-uuid"
	oldService := ServiceData{
		ServiceUUID: &serviceUUID,
		ServiceName: serviceName,
		Misc: []miscData{
			{Key: "ExistingKey", Value: "ExistingValue"},
			{Key: PagerDutyMetadataKey, Value: "ToBeReplaced"},
			{Key: BambooMetadataKey, Value: "ToBeReplaced"},
		},
	}
	expectedMisc := []miscData{
		oldService.Misc[0],
		{Key: PagerDutyMetadataKey, Value: string(pdMeta)},
		{Key: BambooMetadataKey, Value: string(bambooMeta)},
	}

	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{oldService}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, serviceUUID).Return(&oldService, nil)
	serviceCentralClient.On("PatchService", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	store := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err = store.PatchService(context.Background(), testUser, updatedService)

	// then
	require.NoError(t, err)

	serviceCentralClient.AssertNumberOfCalls(t, "PatchService", 1)
	for _, call := range serviceCentralClient.Calls {
		if call.Method != "PatchService" {
			continue
		}

		data, ok := call.Arguments[2].(*ServiceData)
		require.True(t, ok)

		// we only check the misc fields to see if they match
		// need to use ElementsMatch since order is non-deterministic (which is OK)
		assert.ElementsMatch(t, expectedMisc, data.Misc)
	}
}

func TestDeleteServiceDeletesCorrectUUID(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, mock.Anything).Return(expectedData, nil)
	serviceCentralClient.On("DeleteService", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := lister.DeleteService(context.Background(), testUser, testServiceName)

	// then
	require.NoError(t, err)
	serviceCentralClient.AssertCalled(t, "DeleteService", mock.Anything, mock.Anything, *expectedData.ServiceUUID)
}

func TestDeleteServiceTwiceGives404(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, mock.Anything).Return(expectedData, nil)
	serviceCentralClient.On("DeleteService", mock.Anything, mock.Anything, mock.Anything).Return(httputil.NewNotFound("oh no"))
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := lister.DeleteService(context.Background(), testUser, testServiceName)

	// then
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	serviceCentralClient.AssertCalled(t, "DeleteService", mock.Anything, mock.Anything, *expectedData.ServiceUUID)
}

func TestDeleteServiceMissingList(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{}, nil)
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := lister.DeleteService(context.Background(), testUser, testServiceName)

	// then
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	serviceCentralClient.AssertNotCalled(t, "DeleteService", mock.Anything, mock.Anything, mock.Anything)
}

func TestDeleteServiceMissingService(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, mock.Anything).Return(nil, httputil.NewNotFound("oh no"))
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := lister.DeleteService(context.Background(), testUser, testServiceName)

	// then
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	serviceCentralClient.AssertNotCalled(t, "DeleteService", mock.Anything, mock.Anything, mock.Anything)
}

func TestDeleteServiceWeirdResponse(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralClientMock)
	expectedData := newTestServiceData(true)
	serviceCentralClient.On("ListServices", mock.Anything, mock.Anything, mock.Anything).Return([]ServiceData{*expectedData}, nil)
	serviceCentralClient.On("GetService", mock.Anything, mock.Anything, mock.Anything).Return(expectedData, nil)
	serviceCentralClient.On("DeleteService", mock.Anything, mock.Anything, mock.Anything).Return(httputil.NewUnknown("oh no"))
	lister := NewStore(zaptest.NewLogger(t), serviceCentralClient)

	// when
	err := lister.DeleteService(context.Background(), testUser, testServiceName)

	// then
	require.Error(t, err)
	serviceCentralClient.AssertCalled(t, "DeleteService", mock.Anything, mock.Anything, *expectedData.ServiceUUID)
}

func newService(wasCreated bool) *creator_v1.Service {
	service := creator_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: creator_v1.ServiceResourceAPIVersion,
			Kind:       creator_v1.ServiceResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-service",
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: testUser.Name(),
			BusinessUnit:  "PaaS/Micros",
			ResourceTags:  map[voyager.Tag]string{},
		},
	}
	if wasCreated {
		service.CreationTimestamp = meta_v1.NewTime(testCreationTime)
		service.ObjectMeta.UID = types.UID(testServiceUUID)
	}
	return &service
}

func newServiceWithCompliance(wasCreated bool, PRGBEnabled bool) *creator_v1.Service {
	service := creator_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: creator_v1.ServiceResourceAPIVersion,
			Kind:       creator_v1.ServiceResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-service",
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: testUser.Name(),
			BusinessUnit:  "PaaS/Micros",
			ResourceTags:  map[voyager.Tag]string{},
		},
		Status: creator_v1.ServiceStatus{
			Compliance: creator_v1.Compliance{PRGBControl: &PRGBEnabled},
		},
	}
	if wasCreated {
		service.CreationTimestamp = meta_v1.NewTime(testCreationTime)
		service.ObjectMeta.UID = types.UID(testServiceUUID)
	}
	return &service
}

func newServiceServiceCentralData(setID bool) *ServiceData {
	sd := ServiceData{
		ServiceName:          testServiceName,
		ServiceOwner:         ServiceOwner{Username: testUser.Name()},
		ServiceTier:          defaultServiceTier,
		Platform:             voyagerPlatform,
		ZeroDowntimeUpgrades: true,
		Stateless:            true,
		BusinessUnit:         "PaaS/Micros",
	}
	if setID {
		serviceUUID := testServiceUUID
		sd.ServiceUUID = &serviceUUID
		creationTimestamp := testCreationTimestamp
		sd.CreationTimestamp = &creationTimestamp
	}
	return &sd
}
