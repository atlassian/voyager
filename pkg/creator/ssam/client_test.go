package ssam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/atlassian/voyager/pkg/testutil"
	. "github.com/atlassian/voyager/pkg/util/httptest"
	"github.com/atlassian/voyager/pkg/util/httputil"
	pkitestutil "github.com/atlassian/voyager/pkg/util/pkiutil/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseURL(t *testing.T, urlstr string) *url.URL {
	urlobj, err := url.Parse(urlstr)
	require.NoError(t, err)
	return urlobj
}

func TestSSAMClientGetContainerThatExists(t *testing.T) {
	t.Parallel()

	// GIVEN: Load JSON from testdata/
	responseBody, err := testutil.LoadFileFromTestData("get_container_success.json")
	require.NoError(t, err)
	container := &Container{}
	require.NoError(t, json.Unmarshal([]byte(responseBody), container))

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(JSONContent(responseBody)))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	response, err := client.GetContainer(testutil.ContextWithLogger(t), container.ShortName)

	// THEN
	require.NoError(t, err)
	assert.Equal(t, container, response)
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, fmt.Sprintf("/api/access/containers/%s/", container.ShortName), req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientGetContainerThatDoesNotExist(t *testing.T) {
	t.Parallel()

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(
		JSONFromFile(t, "get_container_failed.json"),
		Status(http.StatusNotFound),
	))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	response, err := client.GetContainer(testutil.ContextWithLogger(t), "container-name-that-wont-exist")

	// THEN
	require.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, httputil.IsNotFound(err), fmt.Sprintln(err))
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "/api/access/containers/container-name-that-wont-exist/", req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientPostContainer(t *testing.T) {
	t.Parallel()

	// GIVEN: Load JSON from testdata/
	responseBody, err := testutil.LoadFileFromTestData("post_container_success.json")
	require.NoError(t, err)
	expectedContainer := &Container{}
	require.NoError(t, json.Unmarshal([]byte(responseBody), expectedContainer))

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(
		JSONContent(responseBody),
		Status(http.StatusCreated),
	))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	response, err := client.PostContainer(testutil.ContextWithLogger(t), &ContainerPostRequest{
		SystemOwner:   "sgreenup",
		DisplayName:   "chaos-monkey service",
		ContainerType: "micros",
		ShortName:     "micros-sv--chaos-monkey",
	})

	// THEN
	require.NoError(t, err)
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	assert.Equal(t, expectedContainer, response)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "/api/access/containers/", req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientGetAccessLevel(t *testing.T) {
	t.Parallel()

	// GIVEN: Load JSON from testdata/
	responseBody, err := testutil.LoadFileFromTestData("get_access_level_success.json")
	require.NoError(t, err)
	expectedAccessLevel := &AccessLevel{}
	require.NoError(t, json.Unmarshal([]byte(responseBody), expectedAccessLevel))

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(JSON(t, expectedAccessLevel)))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	response, err := client.GetContainerAccessLevel(
		testutil.ContextWithLogger(t), expectedAccessLevel.System, expectedAccessLevel.ShortName)

	// THEN
	require.NoError(t, err)
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	assert.Equal(t, expectedAccessLevel, response)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodGet, req.Method)
	expectedPath := fmt.Sprintf("/api/access/containers/%s/access-levels/%s/", expectedAccessLevel.System, expectedAccessLevel.ShortName)
	assert.Equal(t, expectedPath, req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientPostAccessLevel(t *testing.T) {
	t.Parallel()

	containerShortName := "micros-sv--chaos-monkey"

	// GIVEN: Load JSON from testdata/
	responseBody, err := testutil.LoadFileFromTestData("post_access_level_success.json")
	require.NoError(t, err)
	expectedAccessLevel := &AccessLevel{}
	require.NoError(t, json.Unmarshal([]byte(responseBody), expectedAccessLevel))

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(
		JSON(t, expectedAccessLevel),
		Status(http.StatusCreated),
	))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	response, err := client.PostAccessLevel(testutil.ContextWithLogger(t), containerShortName, &AccessLevelPostRequest{
		Name:      "Chaos Monkey Administrators",
		ShortName: "chaos-monkey-admins",
		Members: &AccessLevelMembers{
			Users: []string{"sgreenup"},
		},
	})

	// THEN
	require.NoError(t, err)
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	assert.Equal(t, expectedAccessLevel, response)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, fmt.Sprintf("/api/access/containers/%s/access-levels/", containerShortName), req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientDeleteContainerSuccess(t *testing.T) {
	t.Parallel()

	containerShortName := "whatever"

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(
		Status(http.StatusNoContent),
	))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	err := client.DeleteContainer(testutil.ContextWithLogger(t), containerShortName)

	// THEN
	require.NoError(t, err)
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodDelete, req.Method)
	assert.Equal(t, fmt.Sprintf("/api/access/containers/%s/", containerShortName), req.Path)
	assert.Contains(t, req.Header, "Authorization")
}

func TestSSAMClientDeleteContainerNotFound(t *testing.T) {
	t.Parallel()

	containerShortName := "whatever"

	// GIVEN: Setup mock server to respond with testdata/
	handler := MockHandler(Match(AnyRequest).Respond(
		JSONFromFile(t, "delete_container_notfound.json"),
		Status(http.StatusNotFound),
	))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// GIVEN: Setup our SSAM Client
	asapConfig := pkitestutil.MockASAPClientConfig(t)
	client := NewSSAMClient(http.DefaultClient, asapConfig, parseURL(t, srv.URL))

	// WHEN: Make the request
	deleteErr := client.DeleteContainer(testutil.ContextWithLogger(t), containerShortName)

	// THEN
	require.Error(t, deleteErr)
	require.True(t, httputil.IsNotFound(deleteErr))
	assert.Equal(t, handler.ReqestSnapshots.Calls(), 1)
	req := handler.ReqestSnapshots.Snapshots[0]
	assert.Equal(t, http.MethodDelete, req.Method)
	assert.Equal(t, fmt.Sprintf("/api/access/containers/%s/", containerShortName), req.Path)
	assert.Contains(t, req.Header, "Authorization")
}
