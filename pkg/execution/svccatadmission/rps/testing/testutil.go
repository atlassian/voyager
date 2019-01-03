package testing

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/atlassian/voyager/pkg/util"
	. "github.com/atlassian/voyager/pkg/util/httputil/httptest"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func MockRPSCache(t *testing.T) *rps.Cache {
	handler := MockHandler(Match(AnyRequest).Respond(
		Status(http.StatusOK),
		JSONFromFile(t, "list_osb_resources.json"),
	))
	mockServer := httptest.NewServer(handler)
	parsedURL, err := url.Parse(mockServer.URL)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	rpsClient := rps.NewRPSClient(zaptest.NewLogger(t), httpClient, testASAP(t), parsedURL)
	return rps.NewRPSCache(zaptest.NewLogger(t), rpsClient)
}

func testASAP(t *testing.T) *pkiutil.ASAPClientConfig {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return &pkiutil.ASAPClientConfig{
		PrivateKey:   key,
		PrivateKeyID: "test-issuer/test-key",
		Issuer:       "test-issuer",
	}
}
