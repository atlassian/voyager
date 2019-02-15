package it

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/atlassian/voyager/pkg/opsgenie"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	opsgenieIntManURL = "https://micros.prod.atl-paas.net"
)

// NOTE: THIS WILL CREATE INTEGRATIONS IF NONE EXIST
func TestGetIntegrations(t *testing.T) {
	t.Parallel()

	// Prepare ASAP secrets from Kubernetes Secret
	asapCreatorSecret := getSecret(t)
	ctx := testutil.ContextWithLogger(t)
	testLogger := logz.RetrieveLoggerFromContext(ctx)
	asapConfig, asapErr := pkiutil.NewASAPClientConfigFromKubernetesSecret(asapCreatorSecret)
	require.NoError(t, asapErr)

	client := util.HTTPClient()
	c := opsgenie.New(testLogger, client, asapConfig, parseURL(t, opsgenieIntManURL))

	// Get Service Attributes
	resp, _, err := c.GetOrCreateIntegrations(ctx, "Platform SRE")
	require.NoError(t, err)
	require.True(t, len(resp.Integrations) > 0)

	t.Logf("Number of returned integrations: %v", len(resp.Integrations))
	t.Logf("Response: %#v", resp)

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)
	t.Logf("Attributes JSON: %#v", string(bytes))
}

// data should be "export OPSGENIE_YAML=$(kubectl -n voyager get secrets asap-creator -o yaml)"
func getSecret(t *testing.T) *v1.Secret {
	data := os.Getenv("OPSGENIE_YAML") //Envvar containing the yaml contents of the secret

	decode := scheme.Codecs.UniversalDeserializer().Decode
	destination := &v1.Secret{}
	_, _, err := decode([]byte(data), nil, destination)
	require.NoError(t, err)
	return destination
}

func parseURL(t *testing.T, urlstr string) *url.URL {
	urlobj, err := url.Parse(urlstr)
	require.NoError(t, err)
	return urlobj
}
