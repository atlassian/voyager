package httptest

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/atlassian/voyager/pkg/testutil"
	"github.com/stretchr/testify/require"
)

const (
	StatusNext    = 555
	StatusNoMatch = 556
	FailedBody    = "testing mock failed to find matching request handler"
)

// JSONFromFile reads the contents of filename as []bytes writes that along with json content headers
func JSONFromFile(t *testing.T, filename string) Responder {
	data, err := testutil.LoadFileFromTestData(filename)
	require.NoError(t, err)

	return JSONContent(data)
}

// JSON serializes body and writes json content headers
func JSON(t *testing.T, body interface{}) Responder {
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonBody) // nolint
	}
}

// JSONContent should be used when you already have json as []byte
func JSONContent(body []byte) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body) // nolint
	}
}

// Debug can be useful to match specific paths Match(<matchers>).Respond(Debug(t)))
func Debug(t *testing.T) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		t.Log(req.URL.String())
		t.Log(req.Header)
		body, _ := ioutil.ReadAll(req.Body) // nolint
		t.Log(string(body))
		_, _ = io.WriteString(w, "DEBUGGING") // nolint
	}
}

func Status(status int) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(status)
	}
}

func Header(key string, value ...string) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header()[key] = value
	}
}

func Body(body string) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, body) // nolint
	}
}

func BytesBody(body []byte) Responder {
	return func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(body) // nolint
	}
}

type Responder func(w http.ResponseWriter, r *http.Request)

type Responders []Responder

func (m Matchers) Respond(r ...Responder) *MatcherResponder {
	return &MatcherResponder{m, r}
}

func (m Matchers) RespondSeq(r ...Responders) *SeqMatcherResponder {
	return &SeqMatcherResponder{m, r}
}

type SeqMatcherResponder struct {
	m Matchers
	r []Responders
}

func (mr *SeqMatcherResponder) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	for _, matcher := range mr.m {
		if !matcher.Match(r) {
			w.WriteHeader(StatusNext)
			return
		}
	}

	if len(mr.r) > 0 {
		responder := mr.r[0]
		mr.r = mr.r[1:]
		for _, respond := range responder {
			respond(w, r)
		}
	}
}

type MatcherResponder struct {
	m Matchers
	r Responders
}

func (mr *MatcherResponder) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	for _, matcher := range mr.m {
		if !matcher.Match(r) {
			w.WriteHeader(StatusNext)
			return
		}
	}

	// use fake response writer to capture response.WriteHeader(statusCode) calls so we can default it if necessary (when not normally set)
	rr := NewResponseRecorder()
	for _, respond := range mr.r {
		respond(rr, r)
	}

	for k, l := range rr.HeaderMap {
		for _, v := range l {
			w.Header().Set(k, v)
		}
	}
	if rr.Code == 0 {
		rr.Code = http.StatusOK // help simple matchers default to 200 ok
	}
	w.WriteHeader(rr.Code)
	_, _ = w.Write(rr.Body.Bytes()) // nolint
}
