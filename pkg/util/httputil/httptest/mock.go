package httptest

import (
	"io"
	"net/http"
)

type HTTPMock struct {
	handlers         []http.Handler
	RequestSnapshots *RequestSnapshotter
}

func MockHandler(h ...http.Handler) *HTTPMock {
	rs := NewRequestSnapshotter()
	return &HTTPMock{
		handlers:         append([]http.Handler{rs}, h...),
		RequestSnapshots: rs,
	}
}

func (m *HTTPMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	rr := NewResponseRecorder()

	for _, handler := range m.handlers {
		handler.ServeHTTP(rr, r)
		if rr.Code != StatusNext {
			break
		}
	}

	if rr.Code == StatusNext {
		w.WriteHeader(StatusNoMatch)
		_, _ = io.WriteString(w, FailedBody) // nolint
		return
	}

	for k, l := range rr.HeaderMap {
		values := make([]string, len(l))
		copy(values, l)
		w.Header()[k] = values
	}
	w.WriteHeader(rr.Code)
	_, _ = w.Write(rr.Body.Bytes()) // nolint
}
