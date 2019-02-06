package it

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	defaultServiceCentralURL = "https://services.prod.atl-paas.net"
	fakeTestUsername         = "fcobb"
)

func TestGetServiceAttributes(t *testing.T) {
	t.Parallel()

	// Prepare ASAP secrets from Kubernetes Secret
	asapCreatorSecret := getSecret(t)
	ctx := testutil.ContextWithLogger(t)
	testLogger := logz.RetrieveLoggerFromContext(ctx)
	testUser, authErr := auth.Named(fakeTestUsername)
	require.NoError(t, authErr)
	asapConfig, asapErr := pkiutil.NewASAPClientConfigFromKubernetesSecret(asapCreatorSecret)
	require.NoError(t, asapErr)

	client := util.HTTPClient()
	c := servicecentral.NewServiceCentralClient(testLogger, client, asapConfig, parseURL(t, defaultServiceCentralURL))

	// Get Service Attributes
	resp, err := c.GetServiceAttributes(ctx, auth.ToOptionalUser(testUser), "slime")
	require.NoError(t, err)

	t.Logf("Number of returned attributes: %v", len(resp))
	t.Logf("Attributes: %#v", resp)

	bytes, _ := json.Marshal(resp)
	t.Logf("Attributes JSON: %#v", string(bytes))
}

// data should be "export CENTRAL_YAML=$(kubectl -n voyager get secrets asap-creator -o yaml)"
func getSecret(t *testing.T) *core_v1.Secret {
	data := os.Getenv("CENTRAL_YAML") //Envvar containing the yaml contents of the secret

	decode := scheme.Codecs.UniversalDeserializer().Decode
	destination := &core_v1.Secret{}
	_, _, err := decode([]byte(data), nil, destination)
	require.NoError(t, err)
	return destination
}

func parseURL(t *testing.T, urlstr string) *url.URL {
	urlobj, err := url.Parse(urlstr)
	require.NoError(t, err)
	return urlobj
}
