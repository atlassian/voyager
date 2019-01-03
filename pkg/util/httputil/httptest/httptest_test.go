package httptest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryString(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(MockHandler(
		Match(Get, Path("/api/v1/query"), QueryValues("rawr", "rawr", "rawr2")).Respond(Status(200)),
		Match(Get, Path("/api/v1/query"), QueryValues("rawr", "rawr")).Respond(Status(200)),
		Match(Get, Path("/api/v1/query"), Query("blah", "blah2").Add("rawr", "rawr2")).Respond(Status(201)),
	))
	defer s.Close()

	testCases := []struct {
		method  string
		path    string
		status  int
		body    []byte
		headers map[string][]string
	}{
		{"GET", "/api/v1/query?rawr=rawr", 200, []byte{}, map[string][]string{}},
		{"GET", "/api/v1/query?rawr=rawr2&rawr=rawr", 200, []byte{}, map[string][]string{}},
		{"GET", "/api/v1/query?rawr=rawrzers", StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/query?blah=blah2", StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/query", StatusNoMatch, []byte(FailedBody), map[string][]string{}},
	}

	for i := range testCases {
		t.Run(fmt.Sprintf("%s %s", testCases[i].method, testCases[i].path), func(t *testing.T) {
			tc := testCases[i]

			client := &http.Client{}

			req, err := http.NewRequest(tc.method, fmt.Sprintf("%s%s", s.URL, tc.path), nil)
			require.NoError(t, err)
			assert.NotNil(t, req)

			res, err := client.Do(req)
			require.NoError(t, err)
			assert.NotNil(t, res)

			body, err := ioutil.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.status, res.StatusCode)
			assert.Equal(t, tc.body, body)
			for k, v := range tc.headers {
				assert.ElementsMatch(t, v, res.Header[k])
			}
		})
	}
}

func TestJSONof(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(MockHandler(
		Match(Get, Path("/api/v1/exact"), JSONof(t, &map[string]interface{}{
			"test": "kairo",
			"user": "dphan2",
		})).Respond(Status(200)),
	))
	defer s.Close()

	testCases := []struct {
		method  string
		path    string
		reqBody []byte
		status  int
		body    []byte
		headers map[string][]string
	}{
		{"GET", "/api/v1/exact", nil, StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/exact", []byte("{\"test\": \"kairo\", \"user\": \"dphan2\"}"), 200, []byte{}, map[string][]string{}},
		{"GET", "/api/v1/exact", []byte("{\"test\": \"kairo\", \"user\": \"wronguser\"}"), StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/exact", []byte("{\"test\": \"kairo\", \"user\": \"dphan2\", \"id\": 2}"), StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/exact", []byte("{\"test\": \"kairo\", \"user\": \"wronguser\", \"id\": 2}"), StatusNoMatch, []byte(FailedBody), map[string][]string{}},
		{"GET", "/api/v1/exact", []byte("blahblah"), StatusNoMatch, []byte(FailedBody), map[string][]string{}},
	}

	for i := range testCases {
		t.Run(fmt.Sprintf("%s %s %d", testCases[i].method, testCases[i].path, testCases[i].status), func(t *testing.T) {
			tc := testCases[i]

			client := &http.Client{}

			req, err := http.NewRequest(tc.method, fmt.Sprintf("%s%s", s.URL, tc.path), bytes.NewReader(tc.reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-type", "application/json")

			res, err := client.Do(req)
			require.NoError(t, err)

			body, err := ioutil.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.status, res.StatusCode)
			assert.Equal(t, tc.body, body)
			for k, v := range tc.headers {
				assert.ElementsMatch(t, v, res.Header[k])
			}
		})
	}
}

func TestHttp(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(MockHandler(
		Match(Get, Path("/api/v1/foo")).Respond(Status(200)),
		Match(Post, Path("/api/v1/bar")).Respond(Status(201), Body("hello")),
		Match(Post, Path("/api/v1/brunoTheDog")).RespondSeq(
			Responders{
				Status(200),
				Body("fetch"),
			},

			Responders{
				Status(201),
				JSON(t, map[string]interface{}{"dog": 3}),
			},
		),
	))
	defer s.Close()

	defaultHeaders := map[string][]string{
		"Content-Type": {
			"text/plain; charset=utf-8",
		},
	}
	testCases := []struct {
		method  string
		path    string
		status  int
		body    []byte
		headers map[string][]string
	}{
		{"GET", "/api/v1/foo", 200, []byte{}, map[string][]string{}},
		{"POST", "/api/v1/bar", 201, []byte("hello"), defaultHeaders},
		{"POST", "/api/v1/brunoTheDog", 200, []byte("fetch"), defaultHeaders},
		{"POST", "/api/v1/brunoTheDog", 201, []byte("{\"dog\":3}"), map[string][]string{"Content-Type": {"application/json"}}},
		{"POST", "/api/v1/baz", StatusNoMatch, []byte(FailedBody), map[string][]string{}},
	}

	for i := range testCases {
		t.Run(fmt.Sprintf("%s %s", testCases[i].method, testCases[i].path), func(t *testing.T) {
			tc := testCases[i]

			client := &http.Client{}

			req, err := http.NewRequest(tc.method, fmt.Sprintf("%s%s", s.URL, tc.path), nil)
			require.NoError(t, err)
			assert.NotNil(t, req)

			res, err := client.Do(req)
			require.NoError(t, err)
			assert.NotNil(t, res)

			body, err := ioutil.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.status, res.StatusCode)
			assert.Equal(t, tc.body, body)
			for k, v := range tc.headers {
				assert.ElementsMatch(t, v, res.Header[k])
			}
		})
	}
}
