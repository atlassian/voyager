package testing

import (
	"context"
	"net/http"
	"net/http/httptest"
	url2 "net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testLoggingID = "ef4c37b9-e41f-4895-af4d-db27dd9e295c"
)

func TestListServices(t *testing.T) {
	t.Parallel()
	// given
	var calls int
	var methods []string
	var paths []string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// record request details
		calls++
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		// stub response
		w.WriteHeader(http.StatusOK)
		bytes, err := testutil.LoadFileFromTestData("list_osb_resources.json")
		require.NoError(t, err)
		w.Write(bytes)
	}))
	defer mockServer.Close()
	// when
	url, err := url2.Parse(mockServer.URL)
	require.NoError(t, err)
	httpClient := util.HTTPClient()
	rpsClient := rps.NewRPSClient(zaptest.NewLogger(t), httpClient, testASAP(t), url)
	osbResources, err := rpsClient.ListOSBResources(context.Background())
	// then
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, "GET", methods[0])
	assert.Equal(t, "/api/v1/resourceType/osb", paths[0])
	assert.Equal(t, 2, len(osbResources))

	// *something* in the list should be equal
	assert.Subset(t, osbResources, []rps.OSBResource{
		{
			ServiceID:  "node-refapp-jhaggerty-dev",
			InstanceID: "9a3f2d35-0ce8-48b7-8531-a72b5cd02fd4",
		},
	})
}
