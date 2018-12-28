package it

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/creator/ssam"
	"github.com/atlassian/voyager/pkg/pkiutil"
	"github.com/atlassian/voyager/pkg/testutil"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func parseURL(t *testing.T, urlstr string) *url.URL {
	urlobj, err := url.Parse(urlstr)
	require.NoError(t, err)
	return urlobj
}

// data should be "kubectl -n voyager get secrets asap-creator -o yaml"
func getSecret(t *testing.T) *v1.Secret {
	data := `
<INSERT YOUR YAML HERE>
`

	decode := scheme.Codecs.UniversalDeserializer().Decode
	destination := &v1.Secret{}
	_, _, err := decode([]byte(data), nil, destination)
	require.NoError(t, err)
	return destination
}

// This test is tagged in Bazel as manual, and therefore won't be run with the unit-tests. This is useful code for
// manually testing that your code works, not just testing against request/responses saved into test-data. The setup
// for doing this can be a bit tricky, and we don't want to lose the effort that want into it.
//
// Running:
// To make this test work you need to pull down the secret 'asap-creator' from the voyager namespace, and paste it into
// the getSecret function temporarily.
func TestCreateAndDeleteSSAMService(t *testing.T) {
	t.Parallel()

	// Set up our SSAM client and configuration

	asapCreatorSecret := getSecret(t)
	metadata := ssam.ServiceMetadata{
		ServiceName:  "frost-monkey-administrators",
		ServiceOwner: "sgreenup",
	}
	ctx := testutil.ContextWithLogger(t)
	asapConfig, asapErr := pkiutil.NewASAPClientConfigFromKubernetesSecret(asapCreatorSecret)
	require.NoError(t, asapErr)
	client := ssam.NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, "https://ssam.office.atlassian.com"))

	// Create a container
	container, err := client.PostContainer(ctx, &ssam.ContainerPostRequest{
		DisplayName:   metadata.SSAMContainerDisplayName(),
		ShortName:     metadata.SSAMContainerShortName(),
		SystemOwner:   metadata.ServiceOwner,
		ContainerType: "paas",
	})
	require.NoError(t, err)
	require.Equal(t, container.ShortName, metadata.SSAMContainerShortName())

	// The container should now exist
	containerFromGet, getErr := client.GetContainer(ctx, metadata.SSAMContainerShortName())
	require.NoError(t, getErr)
	require.NotNil(t, containerFromGet)
	require.Equal(t, containerFromGet.ShortName, metadata.SSAMContainerShortName())

	// Delete the container
	deleteErr := client.DeleteContainer(ctx, metadata.SSAMContainerShortName())
	require.NoError(t, deleteErr)

	// The container should no longer exist
	_, getAfterDeleteErr := client.GetContainer(ctx, metadata.SSAMContainerShortName())
	require.Error(t, getAfterDeleteErr)
	require.True(t, httputil.IsNotFound(getAfterDeleteErr))
}
