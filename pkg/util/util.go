package util

import (
	"net/http"
	"time"

	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/go-chi/chi"
	chimw "github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

func ToRawExtension(obj interface{}) (*runtime.RawExtension, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, errors.Wrap(err, "unexpectedly failed to marshal JSON")
	}
	return &runtime.RawExtension{
		Raw: data,
	}, nil
}

// DefaultMiddleWare builds an http handler with logging
// fakeServiceName feeds into laas and normally would be a micros service name, here it's a fake
func DefaultMiddleWare(logger *zap.Logger, fakeServiceName string, r *chi.Mux) error {
	logger.Info("Setting up router")

	r.Use(chimw.RequestID)

	// Set up log
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Get the requestID set by chimw.RequestID
			requestID := chimw.GetReqID(req.Context())
			requestLogger := logger.With(
				zap.String("reqID", requestID),
			)

			ctx := logz.CreateContextWithLogger(req.Context(), requestLogger)
			req = req.WithContext(ctx)
			next.ServeHTTP(w, req)
		})
	})

	// Embed the RequestID in the response header
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Get the requestID set by chimw.RequestID
			requestID := chimw.GetReqID(req.Context())
			w.Header().Add("RequestID", requestID)
			next.ServeHTTP(w, req)
		})
	})

	r.Use(httputil.AccessLog(&httputil.ZapAccessLogger{LogContextKey: logz.LoggerContextKey}))
	r.Use(crash.RecoverLogger())
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(render.SetContentType(render.ContentTypeJSON))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		requestLogger := logz.RetrieveLoggerFromContext(r.Context())
		requestLogger.Debug("Unknown request", zap.String("path", r.URL.Path))
		http.Error(w, "Unknown request", http.StatusNotFound)
	})

	return nil
}

// useful in cases where you would write defer thing.Close to work around lint checks for unhandled errors
func CloseSilently(closable interface{ Close() error }) {
	IgnoreError(closable.Close)
}

// useful in cases where you want to document that you explicitly do not want to handle errors
func IgnoreError(f func() error) {
	_ = f() // nolint
}
