package servicecentral

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	. "github.com/atlassian/voyager/pkg/util/httputil/httptest"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/atlassian/voyager/pkg/util/pkiutil/pkitest"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testServiceName       ServiceName = "test-service"
	testServiceUUID                   = "ef4c37b9-e41f-4895-af4d-db27dd9e295c"
	testCreationTimestamp             = "2018-05-17T07:40:48Z"
)

var (
	testUser     = testutil.Named("testuser")
	optionalUser = auth.ToOptionalUser(testUser)
)

func TestCreateNewService(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodPost),
		Path("/api/v1/services"),
		JSONof(t, newTestServiceData(false)),
	).Respond(
		Status(http.StatusCreated),
		JSONFromFile(t, "create_service_rsp.json"),
	))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	_, err := serviceCentralClient.CreateService(context.Background(), testUser, newTestServiceData(false))
	// then
	assert.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
}

func TestCreateServiceFailsIfItAlreadyExists(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(AnyRequest).Respond(Status(http.StatusConflict)))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	_, err := serviceCentralClient.CreateService(context.Background(), testUser, newTestServiceData(false))
	// then
	assert.True(t, httputil.IsConflict(errors.Cause(err)))
}

func TestCreateServiceFailsWhenServiceCentralInternalError(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(AnyRequest).Respond(Status(http.StatusInternalServerError)))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	_, err := serviceCentralClient.CreateService(context.Background(), testUser, newTestServiceData(false))
	// then
	assert.True(t, httputil.IsUnknown(err))
}

func TestUpdateService(t *testing.T) {
	t.Parallel()
	// given
	expectedData := newTestServiceData(true)
	expectedData.ServiceUUID = nil
	expectedData.ServiceName = ""
	handler := MockHandler(Match(
		Method(http.MethodPatch),
		Path("/api/v1/services/"+testServiceUUID),
		JSONof(t, expectedData),
	).Respond(Status(http.StatusOK)))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	testServiceData := newTestServiceData(true)
	copiedTestServiceData := *testServiceData
	err := serviceCentralClient.PatchService(context.Background(), testUser, testServiceData)
	// check that no mutation has occurred
	assert.Equal(t, *testServiceData, copiedTestServiceData)
	// then
	assert.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
}

func TestListServices(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path("/api/v1/services"),
		Query("expand", "tags").AddValues("search", "service_name='test-service'"),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "list_services_rsp.json"),
	))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	serviceData, err := serviceCentralClient.ListServices(context.Background(), optionalUser, "service_name='test-service'")
	// then
	require.NoError(t, err)
	require.Equal(t, 1, len(serviceData))
	require.Equal(t, *newTestServiceData(true), serviceData[0])
}

func TestListModifiedServices(t *testing.T) {
	t.Parallel()
	// given
	now := time.Now()
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path("/api/v2/services"),
		Query("modifiedOn", ">"+now.UTC().Format(time.RFC3339)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "list_modified_services_rsp.json"),
	))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	serviceData, err := serviceCentralClient.ListModifiedServices(context.Background(), optionalUser, now)
	// then
	require.NoError(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
	assert.Equal(t, 1, len(serviceData))
	expected := *newTestServiceData(true)
	expected.Misc = nil // v2 api does not return misc data
	assert.Equal(t, expected, serviceData[0])
}

func TestListServicesPaginates(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(
		Match(
			Method(http.MethodGet),
			Path("/api/v1/services"),
			Query("expand", "tags").
				AddValues("search", "platform='micros2'").
				ExactMatch(),
		).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "list_services_rsp_page1.json"),
		),
		Match(
			Method(http.MethodGet),
			Path("/api/v1/services"),
			Query("expand", "tags").
				AddValues("offset", "1").
				AddValues("search", "platform='micros2'").
				ExactMatch(),
		).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "list_services_rsp_page2.json"),
		),
	)
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	serviceData, err := serviceCentralClient.ListServices(context.Background(), optionalUser, "platform='micros2'")
	// then
	require.NoError(t, err)
	require.Equal(t, 2, handler.RequestSnapshots.Calls())
	require.Equal(t, 2, len(serviceData))
	// cheating here with test data returning the same "service" twice
	require.Equal(t, *newTestServiceData(true), serviceData[0])
	require.Equal(t, *newTestServiceData(true), serviceData[1])
}

func TestDeleteService(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodDelete),
		Path("/api/v1/services/some-uuid"),
	).Respond(Status(http.StatusNoContent)))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	err := serviceCentralClient.DeleteService(context.Background(), testUser, "some-uuid")
	// then
	require.NoError(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
}

func TestDeleteServiceNotFound(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodDelete),
		Path("/api/v1/services/some-uuid"),
	).Respond(
		Status(http.StatusNotFound),
		Body(`{"message":"Service Not Found", "status_code":404}`),
	))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	err := serviceCentralClient.DeleteService(context.Background(), testUser, "some-uuid")
	// then
	require.Error(t, err)

	require.Equal(t, 1, handler.RequestSnapshots.Calls())
}

func TestGetServiceDataFailsWhenServiceCentralInternalError(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(AnyRequest).Respond(Status(http.StatusInternalServerError)))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	user := auth.MaybeNamed("test-owner")
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	_, err := serviceCentralClient.ListServices(context.Background(), user, "test-query")
	// then
	assert.True(t, httputil.IsUnknown(err))
}

func TestGetService(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", v1ServicesPath, testServiceName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_service.rsp.json"),
	))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	_, err := serviceCentralClient.GetService(context.Background(), optionalUser, string(testServiceName))

	// then
	require.NoError(t, err)
	require.Equal(t, 2, handler.RequestSnapshots.Calls())
}

func TestGetServiceWithOpsGenieAttribute(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", v1ServicesPath, testServiceName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_service.rsp.json"),
	),
		Match(
			Method(http.MethodGet),
			Path(fmt.Sprintf("%s/%s/attributes", v2ServicesPath, testServiceName)),
		).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "get_service_attributes.rsp.json"),
		))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	service, err := serviceCentralClient.GetService(context.Background(), optionalUser, string(testServiceName))

	// then
	require.NoError(t, err)
	require.Equal(t, 2, handler.RequestSnapshots.Calls())

	require.Equal(t, 1, len(service.Attributes))
	require.Equal(t, "Platform SRE", service.Attributes[0].Team)
}

func TestGetServiceWithEmptyOpsGenieAttribute(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", v1ServicesPath, testServiceName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_service.rsp.json"),
	),
		Match(
			Method(http.MethodGet),
			Path(fmt.Sprintf("%s/%s/attributes", v2ServicesPath, testServiceName)),
		).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "get_service_attributes_empty_team_string.rsp.json"),
		))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	service, err := serviceCentralClient.GetService(context.Background(), optionalUser, string(testServiceName))

	// then
	require.NoError(t, err)
	require.Equal(t, 2, handler.RequestSnapshots.Calls())

	require.Equal(t, 1, len(service.Attributes))
	require.Equal(t, "", service.Attributes[0].Team)
}

func TestGetServiceWithoutOpsGenieAttribute(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", v1ServicesPath, testServiceName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_service.rsp.json"),
	),
		Match(
			Method(http.MethodGet),
			Path(fmt.Sprintf("%s/%s/attributes", v2ServicesPath, testServiceName)),
		).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "get_service_attributes_empty.rsp.json"),
		))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	service, err := serviceCentralClient.GetService(context.Background(), optionalUser, string(testServiceName))

	// then
	require.NoError(t, err)
	require.Equal(t, 2, handler.RequestSnapshots.Calls())
	require.Equal(t, 0, len(service.Attributes))
}

func TestGetServiceWithFailedAttributesCall(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", v1ServicesPath, testServiceName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_service.rsp.json"),
	),
		Match(
			Method(http.MethodGet),
			Path(fmt.Sprintf("%s/%s/attributes", v2ServicesPath, testServiceName)),
		).Respond(
			Status(http.StatusInternalServerError),
		))
	serviceCentralServerMock := httptest.NewServer(handler)
	defer serviceCentralServerMock.Close()
	// when
	serviceCentralClient := testServiceCentralClient(t, serviceCentralServerMock.URL, pkitest.MockASAPClientConfig(t))
	service, err := serviceCentralClient.GetService(context.Background(), optionalUser, string(testServiceName))

	// then
	require.NoError(t, err) // Expect no error as OpsGenie team is optional
	require.Equal(t, 2, handler.RequestSnapshots.Calls())
	require.Equal(t, 0, len(service.Attributes))
}

func testServiceCentralClient(t *testing.T, serviceCentralServerMockAddress string, asap pkiutil.ASAP) *Client {
	serviceCentralURL, err := url.Parse(serviceCentralServerMockAddress)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	return NewServiceCentralClient(zaptest.NewLogger(t), httpClient, asap, serviceCentralURL)
}

// should match data from create_service_rsp.json and list_services_rsp.json
func newTestServiceData(setID bool) *ServiceData {
	s := ServiceData{
		ServiceName:          testServiceName,
		ServiceOwner:         ServiceOwner{Username: testUser.Name()},
		ServiceTier:          3,
		Platform:             "micros2",
		ZeroDowntimeUpgrades: true,
		Stateless:            true,
		BusinessUnit:         "some_unit",
		Misc: []miscData{
			{
				Key:   "testKey",
				Value: "testValue",
			},
		},
	}
	if setID {
		serviceUUID := testServiceUUID
		s.ServiceUUID = &serviceUUID
		creationTimestamp := testCreationTimestamp
		s.CreationTimestamp = &creationTimestamp
	}
	return &s
}
