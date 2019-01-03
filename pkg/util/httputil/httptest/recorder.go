package httptest

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
)

type RequestSnapshot struct {
	Method string
	Path   string
	Query  string
	Header map[string]string
}

func NewRequestSnapshot(req *http.Request) *RequestSnapshot {
	headers := map[string]string{}
	for k := range req.Header {
		// req.Header has a weird way of storing the values associated to a key, so use Get()
		headers[k] = req.Header.Get(k)
	}

	return &RequestSnapshot{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  req.URL.RawQuery,
		Header: headers,
	}
}

type RequestSnapshotter struct {
	sync.Mutex
	Snapshots []*RequestSnapshot
}

func NewRequestSnapshotter() *RequestSnapshotter {
	return &RequestSnapshotter{}
}

func (s *RequestSnapshotter) Add(req *http.Request) {
	s.Lock()
	defer s.Unlock()
	s.Snapshots = append(s.Snapshots, NewRequestSnapshot(req))
}

func (s *RequestSnapshotter) Debug(t *testing.T) {
	s.Lock()
	defer s.Unlock()
	for _, snapShot := range s.Snapshots {
		t.Log(*snapShot)
	}
}

func (s *RequestSnapshotter) Calls() int {
	s.Lock()
	defer s.Unlock()
	return len(s.Snapshots)
}

type ResponseRecorder struct {
	Code      int
	HeaderMap http.Header
	Body      *bytes.Buffer
}

func NewResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		Code:      http.StatusOK,
		HeaderMap: make(http.Header),
		Body:      bytes.NewBuffer(nil),
	}
}

func (rr *ResponseRecorder) Header() http.Header {
	return rr.HeaderMap
}

func (rr *ResponseRecorder) Write(p []byte) (int, error) {
	rr.Body.Truncate(0)
	return rr.Body.Write(p)
}

func (rr *ResponseRecorder) WriteHeader(code int) {
	rr.Code = code
}
