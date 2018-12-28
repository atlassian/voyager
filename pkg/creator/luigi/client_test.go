package luigi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/pkiutil"
	"github.com/atlassian/voyager/pkg/util"
	. "github.com/atlassian/voyager/pkg/util/httptest"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testLoggingID = "ef4c37b9-e41f-4895-af4d-db27dd9e295c"
)

func TestCreateNewService(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(Match(Post, Path("/api/v1/services"), JSONof(t, newTestServiceData(false))).Respond(
		Status(http.StatusCreated),
		JSONFromFile(t, "create_service_rsp.json"),
	))
	luigiServerMock := httptest.NewServer(handler)
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	_, err := luigiClient.CreateService(context.Background(), newTestServiceData(false))
	// then
	assert.NoError(t, err)
	require.Equal(t, 1, handler.ReqestSnapshots.Calls())
}

func TestDeleteService(t *testing.T) {
	t.Parallel()
	// given
	loggingID := "id"
	handler := MockHandler(Match(Delete, Path("/api/v1/services/"+loggingID), NoBody).Respond(
		Status(http.StatusOK),
		Body(`{"data":[],"status_code":200,"message":"deleted"}`),
	))
	luigiServerMock := httptest.NewServer(handler)
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	err := luigiClient.DeleteService(context.Background(), loggingID)
	// then
	assert.NoError(t, err)
	require.Equal(t, 1, handler.ReqestSnapshots.Calls())
}

func TestCreateServiceFailsIfItAlreadyExists(t *testing.T) {
	t.Parallel()
	// given
	luigiServerMock := httptest.NewServer(Match(AnyRequest).Respond(
		Status(http.StatusConflict),
		Body(`{"data":[],"status_code":409,"errors":[{"message":"Error a service exists already with that Name and SourceID"}]}`),
	))
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	_, err := luigiClient.CreateService(context.Background(), newTestServiceData(false))
	// then
	require.IsTypef(t, &httputil.ClientError{}, errors.Cause(err), "contents: %q", err)
	assert.True(t, httputil.IsConflict(err))
}

func TestCreateServiceFailsWhenLuigiInternalError(t *testing.T) {
	t.Parallel()
	// given
	luigiServerMock := httptest.NewServer(Match(AnyRequest).Respond(
		Status(http.StatusInternalServerError),
		Body(`{"data":[],"status_code":500,"errors":[{"message":"Death"}]}`),
	))
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	_, err := luigiClient.CreateService(context.Background(), newTestServiceData(false))
	// then
	require.IsTypef(t, &httputil.ClientError{}, errors.Cause(err), "contents: %q", err)
	assert.True(t, httputil.IsUnknown(err))
}

func TestListServices(t *testing.T) {
	t.Parallel()
	// given
	handler := MockHandler(
		Match(Get, Path("/api/v1/services"), Query("search", url.QueryEscape("test-serv&ice"))).Respond(
			Status(http.StatusOK),
			JSONFromFile(t, "list_services_rsp.json"),
		))
	luigiServerMock := httptest.NewServer(handler)
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	serviceData, err := luigiClient.ListServices(context.Background(), "test-serv&ice")
	// then
	assert.NoError(t, err)
	require.Equal(t, 1, handler.ReqestSnapshots.Calls())
	assert.Equal(t, 9, len(serviceData))

	// *something* in the list should requal
	assert.Contains(t, serviceData, newTestServiceData(true).BasicServiceData)
}

func TestGetServiceDataFailsWhenLuigiInternalError(t *testing.T) {
	t.Parallel()
	// given
	luigiServerMock := httptest.NewServer(Match(AnyRequest).Respond(
		Status(http.StatusInternalServerError),
		Body(`{"data":[],"status_code":500,"errors":[{"message":"OMG"}]}`),
	))
	defer luigiServerMock.Close()
	// when
	luigiClient := testLuigiClient(t, luigiServerMock.URL, testASAP(t))
	_, err := luigiClient.ListServices(context.Background(), "test-service")
	// then
	assert.True(t, httputil.IsUnknown(err))
}

func testASAP(t *testing.T) *pkiutil.ASAPClientConfig {
	key, err := rsa.GenerateKey(rand.Reader, 512)
	require.NoError(t, err)
	return &pkiutil.ASAPClientConfig{
		PrivateKey:   key,
		PrivateKeyID: "test-issuer/test-key",
		Issuer:       "test-issuer",
	}
}

func testLuigiClient(t *testing.T, luigiServerMockAddress string, asap pkiutil.ASAP) *ClientImpl {
	luigiURL, err := url.Parse(luigiServerMockAddress)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	return NewLuigiClient(zaptest.NewLogger(t), httpClient, asap, luigiURL)
}

// should match data from create_service_rsp.json and list_services_rsp.json
func newTestServiceData(setID bool) *FullServiceData {
	s := FullServiceData{
		BasicServiceData: BasicServiceData{
			SourceID:          "micros2",
			Name:              "test-service",
			Organization:      "some_unit",
			Owner:             "an_owner",
			Admins:            "",
			CapacityGigabytes: 1,
			CapacityComment:   "shibe",
		},
		Acls: []ServiceACL{
			ServiceACL{Environments: "*", StaffIDGroup: "atlassian-staff"},
		},
	}
	if setID {
		s.LoggingID = testLoggingID
	}
	return &s
}
