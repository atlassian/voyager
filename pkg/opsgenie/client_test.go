package opsgenie

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/util"
	. "github.com/atlassian/voyager/pkg/util/httputil/httptest"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/atlassian/voyager/pkg/util/pkiutil/pkitest"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestGetIntegrations(t *testing.T) {
	t.Parallel()

	const teamName = "Platform SRE"

	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", integrationsPath, teamName)),
	).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "get_or_create_integrations.rsp.json"),
	))
	ogIntManagerMockServer := httptest.NewServer(handler)
	defer ogIntManagerMockServer.Close()

	// when
	ogIntManagerClient := mockOpsGenieIntegrationManagerClient(t, ogIntManagerMockServer.URL, pkitest.MockASAPClientConfig(t))
	_, retriable, err := ogIntManagerClient.GetOrCreateIntegrations(context.Background(), teamName)

	// then
	require.NoError(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
	require.False(t, retriable)
}

func TestGetIntegrationsTeamNotFound(t *testing.T) {
	t.Parallel()

	const teamName = "Platform SRE"

	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", integrationsPath, teamName)),
	).Respond(
		Status(http.StatusNotFound),
	))
	ogIntManagerMockServer := httptest.NewServer(handler)
	defer ogIntManagerMockServer.Close()

	// when
	ogIntManagerClient := mockOpsGenieIntegrationManagerClient(t, ogIntManagerMockServer.URL, pkitest.MockASAPClientConfig(t))
	_, retriable, err := ogIntManagerClient.GetOrCreateIntegrations(context.Background(), teamName)

	// then
	require.Error(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
	require.False(t, retriable)
}

func TestGetIntegrationsRateLimited(t *testing.T) {
	t.Parallel()

	const teamName = "Platform SRE"

	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", integrationsPath, teamName)),
	).Respond(
		Status(http.StatusTooManyRequests),
	))
	ogIntManagerMockServer := httptest.NewServer(handler)
	defer ogIntManagerMockServer.Close()

	// when
	ogIntManagerClient := mockOpsGenieIntegrationManagerClient(t, ogIntManagerMockServer.URL, pkitest.MockASAPClientConfig(t))
	_, retriable, err := ogIntManagerClient.GetOrCreateIntegrations(context.Background(), teamName)

	// then
	require.Error(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
	require.True(t, retriable)
}

func TestGetIntegrationsInternalServerError(t *testing.T) {
	t.Parallel()

	const teamName = "Platform SRE"

	// given
	handler := MockHandler(Match(
		Method(http.MethodGet),
		Path(fmt.Sprintf("%s/%s", integrationsPath, teamName)),
	).Respond(
		Status(http.StatusInternalServerError),
	))
	ogIntManagerMockServer := httptest.NewServer(handler)
	defer ogIntManagerMockServer.Close()

	// when
	ogIntManagerClient := mockOpsGenieIntegrationManagerClient(t, ogIntManagerMockServer.URL, pkitest.MockASAPClientConfig(t))
	_, retriable, err := ogIntManagerClient.GetOrCreateIntegrations(context.Background(), teamName)

	// then
	require.Error(t, err)
	require.Equal(t, 1, handler.RequestSnapshots.Calls())
	require.True(t, retriable)
}

func mockOpsGenieIntegrationManagerClient(t *testing.T, serverMockAddress string, asap pkiutil.ASAP) *Client {
	opsgenieIntegrationManagerURL, err := url.Parse(serverMockAddress)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	return New(zaptest.NewLogger(t), httpClient, asap, opsgenieIntegrationManagerURL)
}
