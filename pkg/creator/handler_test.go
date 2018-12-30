package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/creator/luigi"
	"github.com/atlassian/voyager/pkg/creator/ssam"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/testutil"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

const (
	testServiceName       = "per-tenant-loop"
	testBusinessUnit      = "some_unit"
	testSSAMContainerName = "paas-" + testServiceName
	testPagerDutyURL      = "https://atlassian.pagerduty.com/services#?query=Micros%202%20-%20per-tenant-loop"
	testLoggingID         = "ef4c37b9-e41f-4895-af4d-db27dd9e295c"
	testAccessLevel       = "test-ssam-access-level"
)

var (
	testResourceOwner = testutil.Named("an_owner")
	testEmail         = testResourceOwner.Name() + "@atlassian.com"

	_ ServiceCentralStoreInterface = &servicecentral.Store{}
	_ PagerDutyClientInterface     = &pagerduty.Client{}
	_ LuigiClientInterface         = &luigi.Creator{}
	_ SSAMClientInterface          = &ssam.ServiceCreator{}
	_ ServiceCentralStoreInterface = &serviceCentralMock{}
	_ PagerDutyClientInterface     = &pagerDutyMock{}
)

// Service Central

type serviceCentralMock struct {
	mock.Mock
}

func (m *serviceCentralMock) FindOrCreateService(ctx context.Context, user auth.User, service *creator_v1.Service) (*creator_v1.Service, error) {
	args := m.Called(ctx, user, service)
	result := args.Get(0)
	err := args.Error(1)
	if result != nil {
		return result.(*creator_v1.Service), err
	} else {
		return nil, err
	}
}

func (m *serviceCentralMock) GetService(ctx context.Context, user auth.OptionalUser, name string) (*creator_v1.Service, error) {
	args := m.Called(ctx, user, name)
	svc, _ := args.Get(0).(*creator_v1.Service)
	return svc, args.Error(1)
}

func (m *serviceCentralMock) ListServices(ctx context.Context, user auth.OptionalUser) ([]creator_v1.Service, error) {
	args := m.Called(ctx, user)
	return args.Get(0).([]creator_v1.Service), args.Error(1)
}

func (m *serviceCentralMock) ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]creator_v1.Service, error) {
	args := m.Called(ctx, user, modifiedSince)
	return args.Get(0).([]creator_v1.Service), args.Error(1)
}

func (m *serviceCentralMock) PatchService(ctx context.Context, user auth.User, service *creator_v1.Service) error {
	args := m.Called(ctx, user, service)
	return args.Error(0)
}

func (m *serviceCentralMock) DeleteService(ctx context.Context, user auth.User, name string) error {
	args := m.Called(ctx, user, name)
	return args.Error(0)
}

// PagerDuty

type pagerDutyMock struct {
	mock.Mock
}

func (m *pagerDutyMock) FindOrCreate(serviceName string, user auth.User, email string) (creator_v1.PagerDutyMetadata, error) {
	args := m.Called(serviceName, user, email)
	return args.Get(0).(creator_v1.PagerDutyMetadata), args.Error(1)
}

func (m *pagerDutyMock) Delete(serviceName string) error {
	args := m.Called(serviceName)
	return args.Error(0)
}

// Luigi

type luigiMock struct {
	mock.Mock
}

var _ LuigiClientInterface = &luigiMock{}

func (m *luigiMock) FindOrCreateService(ctx context.Context, meta *luigi.ServiceMetadata) (*luigi.BasicServiceData, error) {
	args := m.Called(ctx, meta)
	return args.Get(0).(*luigi.BasicServiceData), args.Error(1)
}

func (m *luigiMock) DeleteService(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// SSAM

type ssamMock struct {
	mock.Mock
}

var _ SSAMClientInterface = &ssamMock{}

func (m *ssamMock) GetExpectedServiceContainerName(ctx context.Context, metadata *ssam.ServiceMetadata) string {
	args := m.Called(ctx, metadata)
	return args.Get(0).(string)
}

func (m *ssamMock) CreateService(ctx context.Context, metadata *ssam.ServiceMetadata) (string, ssam.AccessLevels, error) {
	args := m.Called(ctx, metadata)
	return args.Get(0).(string), ssam.AccessLevels{
		Production: testAccessLevel,
	}, args.Error(1)
}

func (m *ssamMock) DeleteService(ctx context.Context, metadata *ssam.ServiceMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()
	// given
	serviceCentralClient := new(serviceCentralMock)
	pagerDutyClient := new(pagerDutyMock)
	luigiClient := new(luigiMock)
	ssamClient := new(ssamMock)
	handler := ServiceHandler{
		serviceCentral: serviceCentralClient,
		pagerDuty:      pagerDutyClient,
		luigi:          luigiClient,
		ssam:           ssamClient,
	}
	logger := zaptest.NewLogger(t)

	serviceCentralClient.On("FindOrCreateService", mock.Anything, testResourceOwner, testService(testServiceName)).Return(testService(testServiceName), nil)
	serviceCentralClient.On("PatchService", mock.Anything, testResourceOwner, testServiceWithMetadata()).Return(nil)
	ssamClient.On("CreateService", mock.Anything, testSSAMServiceMetadata()).Return(testSSAMContainerName, nil)
	pagerDutyClient.On("FindOrCreate", testServiceName, testResourceOwner, testEmail).Return(*testPagerDutyMetadata(), nil)
	luigiClient.On("FindOrCreateService", mock.Anything, testLuigiServiceMetadata()).Return(testLuigiData(), nil)

	// when
	reqSrc, err := testutil.LoadFileFromTestData("service_create.json")
	require.NoError(t, err)
	service := creator_v1.Service{}
	err = json.Unmarshal(reqSrc, &service)
	require.NoError(t, err)
	ctx := context.Background()
	ctx = request.WithUser(ctx, userInfo(testResourceOwner.Name()))
	ctx = logz.CreateContextWithLogger(ctx, logger)

	newService, err := handler.ServiceCreate(ctx, &service)

	// then
	require.NoError(t, err)
	actualBytes, err := json.Marshal(newService)
	expectedBytes, err := json.Marshal(testServiceWithMetadata())
	assert.Equal(t, expectedBytes, actualBytes)
}

func TestServiceCreatePropagatesBadRequestError(t *testing.T) {
	t.Parallel()
	// given
	serviceCentralClient := new(serviceCentralMock)
	pagerDutyClient := new(pagerDutyMock)
	luigiClient := new(luigiMock)
	ssamClient := new(ssamMock)
	handler := ServiceHandler{
		serviceCentral: serviceCentralClient,
		pagerDuty:      pagerDutyClient,
		luigi:          luigiClient,
		ssam:           ssamClient,
	}
	logger := zaptest.NewLogger(t)

	serviceCentralClient.On("FindOrCreateService", mock.Anything, testResourceOwner, testService(testServiceName)).Return(nil, httputil.NewBadRequest(""))

	// when
	reqSrc, err := testutil.LoadFileFromTestData("service_create.json")
	require.NoError(t, err)
	service := creator_v1.Service{}
	err = json.Unmarshal(reqSrc, &service)
	require.NoError(t, err)
	ctx := context.Background()
	ctx = request.WithUser(ctx, userInfo(testResourceOwner.Name()))
	ctx = logz.CreateContextWithLogger(ctx, logger)

	newService, err := handler.ServiceCreate(ctx, &service)

	// then
	assert.Nil(t, newService)
	assert.NotNil(t, err)
	assert.True(t, apierrors.IsBadRequest(err))
}

func TestServiceCreateDoubleDashServiceName(t *testing.T) {
	t.Parallel()

	// given
	service := testService("hello--world")

	// when
	err := defaultAndValidateService(service, testResourceOwner)

	// then
	require.Error(t, err)
}

func TestServiceCreateBlacklistedServiceName(t *testing.T) {
	t.Parallel()

	for _, badword := range blacklist {
		t.Run(badword, func(t *testing.T) {
			// given
			service := testService(badword)

			// when
			err := defaultAndValidateService(service, testResourceOwner)

			// then
			require.Error(t, err)
			require.True(t, IsBlackListError(err))
		})
	}
}

func TestServiceCreateNonBlacklistedServiceName(t *testing.T) {
	t.Parallel()
	var whiteList []string
	for _, badWord := range blacklist {
		whiteList = append(whiteList, fmt.Sprintf("%s-test", badWord))
	}

	for _, goodWord := range whiteList {
		t.Run(goodWord, func(t *testing.T) {
			// given
			service := testService(goodWord)

			// when
			err := defaultAndValidateService(service, testResourceOwner)

			// then
			require.NoError(t, err)
		})
	}
}

func TestServiceCreateDefaultOwnerWhenMissing(t *testing.T) {
	t.Parallel()

	// given
	service := testService("creator-unit-test")
	service.Spec.ResourceOwner = ""

	// when
	parseErr := defaultAndValidateService(service, testResourceOwner)

	// then
	assert.NoError(t, parseErr)
	assert.Equal(t, service.Spec.ResourceOwner, testResourceOwner.Name())
}

func TestServiceSkipDefaultOwnerWhenOwnerPresent(t *testing.T) {
	t.Parallel()

	// given
	owner := "someonewhoisnotvcreator"
	service := testService("creator-unit-test")
	service.Spec.ResourceOwner = owner

	// when
	parseErr := defaultAndValidateService(service, testResourceOwner)

	// then
	assert.NoError(t, parseErr)
	assert.Equal(t, service.Spec.ResourceOwner, owner)
}

func TestServiceDelete(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralMock)
	pagerDutyClient := new(pagerDutyMock)
	luigiClient := new(luigiMock)
	ssamClient := new(ssamMock)
	handler := ServiceHandler{
		serviceCentral: serviceCentralClient,
		pagerDuty:      pagerDutyClient,
		luigi:          luigiClient,
		ssam:           ssamClient,
	}
	logger := zaptest.NewLogger(t)

	testService := testService(testServiceName)
	serviceCentralClient.On("GetService", mock.Anything, auth.ToOptionalUser(testResourceOwner), testServiceName).Return(testService, nil)
	luigiClient.On("DeleteService", mock.Anything, testService.Spec.LoggingID).Return(nil)
	pagerDutyClient.On("Delete", testServiceName).Return(nil)
	ssamClient.On("DeleteService", mock.Anything, &ssam.ServiceMetadata{
		ServiceName:  testServiceName,
		ServiceOwner: testService.Spec.ResourceOwner,
	}).Return(nil)
	serviceCentralClient.On("DeleteService", mock.Anything, testResourceOwner, testServiceName).Return(nil)

	// when
	ctx := context.Background()
	ctx = request.WithUser(ctx, userInfo(testResourceOwner.Name()))
	ctx = logz.CreateContextWithLogger(ctx, logger)

	_, err := handler.ServiceDelete(ctx, testServiceName)

	// then
	require.NoError(t, err)
}

func TestServiceDeleteNotFound(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralMock)
	pagerDutyClient := new(pagerDutyMock)
	luigiClient := new(luigiMock)
	ssamClient := new(ssamMock)
	handler := ServiceHandler{
		serviceCentral: serviceCentralClient,
		pagerDuty:      pagerDutyClient,
		luigi:          luigiClient,
		ssam:           ssamClient,
	}
	logger := zaptest.NewLogger(t)

	testService := testService(testServiceName)
	serviceCentralClient.On("GetService", mock.Anything, auth.ToOptionalUser(testResourceOwner), testServiceName).Return(testService, nil)
	luigiClient.On("DeleteService", mock.Anything, testService.Spec.LoggingID).Return(httputil.NewNotFound("nope"))
	pagerDutyClient.On("Delete", testServiceName).Return(httputil.NewNotFound("nope"))
	ssamClient.On("DeleteService", mock.Anything, &ssam.ServiceMetadata{
		ServiceName:  testServiceName,
		ServiceOwner: testService.Spec.ResourceOwner,
	}).Return(httputil.NewNotFound("nope"))
	serviceCentralClient.On("DeleteService", mock.Anything, testResourceOwner, testServiceName).Return(nil)

	// when
	ctx := context.Background()
	ctx = request.WithUser(ctx, userInfo(testResourceOwner.Name()))
	ctx = logz.CreateContextWithLogger(ctx, logger)

	_, err := handler.ServiceDelete(ctx, testServiceName)

	// then
	require.NoError(t, err)
}

func TestServiceDeleteActualServiceNotFound(t *testing.T) {
	t.Parallel()

	// given
	serviceCentralClient := new(serviceCentralMock)
	pagerDutyClient := new(pagerDutyMock)
	luigiClient := new(luigiMock)
	ssamClient := new(ssamMock)
	handler := ServiceHandler{
		serviceCentral: serviceCentralClient,
		pagerDuty:      pagerDutyClient,
		luigi:          luigiClient,
		ssam:           ssamClient,
	}
	logger := zaptest.NewLogger(t)

	serviceCentralClient.On("GetService", mock.Anything, auth.ToOptionalUser(testResourceOwner), testServiceName).Return(nil, servicecentral.NewNotFound("not found"))

	// when
	ctx := context.Background()
	ctx = request.WithUser(ctx, userInfo(testResourceOwner.Name()))
	ctx = logz.CreateContextWithLogger(ctx, logger)

	_, err := handler.ServiceDelete(ctx, testServiceName)

	// then
	assert.NotNil(t, err)
	assert.True(t, apierrors.IsNotFound(err))
}

func testService(name string) *creator_v1.Service {
	return &creator_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       creator_v1.ServiceResourceKind,
			APIVersion: creator_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name,
		},
		Spec: creator_v1.ServiceSpec{
			ResourceOwner: testResourceOwner.Name(),
			BusinessUnit:  testBusinessUnit,
		},
	}
}

func testServiceWithMetadata() *creator_v1.Service {
	service := testService(testServiceName)
	service.Spec.SSAMContainerName = testSSAMContainerName
	service.Spec.PagerDutyServiceID = testPagerDutyURL
	service.Spec.LoggingID = testLoggingID
	service.Spec.Metadata = creator_v1.ServiceMetadata{
		PagerDuty: testPagerDutyMetadata(),
	}
	return service
}

func testPagerDutyMetadata() *creator_v1.PagerDutyMetadata {
	return &creator_v1.PagerDutyMetadata{}
}

func testLuigiServiceMetadata() *luigi.ServiceMetadata {
	return &luigi.ServiceMetadata{
		Name:         testServiceName,
		BusinessUnit: testBusinessUnit,
		Owner:        testResourceOwner.Name(),
		Admins:       testAccessLevel,
	}
}

func testLuigiData() *luigi.BasicServiceData {
	return &luigi.BasicServiceData{
		LoggingID: testLoggingID,
	}
}

func testSSAMServiceMetadata() *ssam.ServiceMetadata {
	return &ssam.ServiceMetadata{
		ServiceName:  testServiceName,
		ServiceOwner: testResourceOwner.Name(),
	}
}

func TestValidateServiceName(t *testing.T) {
	t.Parallel()

	testCases := []string{
		strings.Repeat("a", ServiceNameMaximumLength),
		"my-test-svc",
		"my-testing-service",
		"alpha",
		"a",
		"nope.com",
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			require.NoError(t, validateServiceName(tc))
		})
	}
}

func TestValidateServiceNameInvalid(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"",
		"!",
		"?!%20",
		"...",
		"../../..",
		"../",
		"..",
		"{}",
		"http://nope.com/",
		"-",
		"a-",
		"a-b-",
		"-a-b",
		"-a",
		strings.Repeat("a", ServiceNameMaximumLength+1),
		strings.Repeat("a", ServiceNameMaximumLength+2),
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			require.Error(t, validateServiceName(tc))
		})
	}
}

func TestValidateServiceNameBlacklist(t *testing.T) {
	t.Parallel()

	require.True(t, len(blacklist) > 1)
	for _, serviceName := range blacklist {
		err := validateServiceName(serviceName)
		require.Error(t, err)
	}
}

func userInfo(username string) user.Info {
	return &user.DefaultInfo{
		Name:   username,
		Groups: []string{"grp1"},
	}
}
