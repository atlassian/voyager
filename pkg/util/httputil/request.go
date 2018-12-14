package httputil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
)

type AccessLogger interface {
	AccessLog(ctx context.Context, req AccessRequest, res AccessResponse, dur time.Duration)
}

type AccessRequest interface {
	RequestID() string
	RemoteAddr() string
	Proto() string
	Method() string
	RequestURI() string
	Host() string
	Referer() string
}

type AccessResponse interface {
	Status() int
	BytesWritten() int
}

func AccessLog(l AccessLogger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			t0 := time.Now()
			next.ServeHTTP(res, r)
			t1 := time.Now()
			tn := t1.Sub(t0)

			reqID := chimw.GetReqID(r.Context())
			if reqID == "" {
				reqID = "-"
			}

			req := accessRequest{
				id:  reqID,
				req: r,
			}
			l.AccessLog(r.Context(), &req, res, tn)
		})
	}
}

type accessRequest struct {
	id  string
	req *http.Request
}

func (r *accessRequest) RequestID() string {
	return r.id
}

func (r *accessRequest) RemoteAddr() string {
	return r.req.RemoteAddr
}

func (r *accessRequest) Proto() string {
	return r.req.Proto
}

func (r *accessRequest) Method() string {
	return r.req.Method
}

func (r *accessRequest) RequestURI() string {
	return r.req.RequestURI
}

func (r *accessRequest) Host() string {
	return r.req.Host
}

func (r *accessRequest) Referer() string {
	return r.req.Referer()
}

var (
	zapKindAccess = zap.String("kind", "access")
)

type ZapAccessLogger struct {
	LogContextKey interface{}
}

func (l *ZapAccessLogger) AccessLog(ctx context.Context, req AccessRequest, res AccessResponse, dur time.Duration) {
	log, ok := ctx.Value(l.LogContextKey).(*zap.Logger)
	if !ok {
		panic(fmt.Sprintf("ZapAccessLogger's LogContextKey: `%v` doesn't point to a zap.Logger!", l.LogContextKey))
	}
	log.Info("Access",
		zapKindAccess,
		zap.String("request_reqid", req.RequestID()),
		zap.String("request_remote", req.RemoteAddr()),
		zap.String("request_proto", req.Proto()),
		zap.String("request_method", req.Method()),
		zap.String("request_url", req.RequestURI()),
		zap.String("request_host", req.Host()),
		zap.String("request_referer", req.Referer()),
		zap.Int("responses_status", res.Status()),
		zap.Int("response_length", res.BytesWritten()),
		zap.Duration("duration", dur),
	)
}
