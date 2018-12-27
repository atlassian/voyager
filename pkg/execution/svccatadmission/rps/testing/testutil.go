package testing

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	url2 "net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/atlassian/voyager/pkg/pkiutil"
	"github.com/atlassian/voyager/pkg/util"
	httptest2 "github.com/atlassian/voyager/pkg/util/httptest"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func MockRPSCache(t *testing.T) *rps.Cache {
	handler := httptest2.MockHandler(httptest2.Match(httptest2.AnyRequest).Respond(
		httptest2.Status(http.StatusOK),
		httptest2.JSONFromFile(t, "list_osb_resources.json"),
	))
	mockServer := httptest.NewServer(handler)
	url, err := url2.Parse(mockServer.URL)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	rpsClient := rps.NewRPSClient(zaptest.NewLogger(t), httpClient, testASAP(t), url)
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
