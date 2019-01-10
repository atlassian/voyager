package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/atlassian/ctrl/process"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/util/tlsutil"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultShutdownTimeout = 15 * time.Second
)

type HTTPServer struct {
	serviceName string
	logger      *zap.Logger
	router      *chi.Mux
	config      options.ServerConfig
}

func NewHTTPServer(serviceName string, logger *zap.Logger, config options.ServerConfig) (*HTTPServer, error) {
	r, err := NewRouter(serviceName, logger)
	if err != nil {
		return nil, err
	}

	return &HTTPServer{
		serviceName: serviceName,
		logger:      logger,
		router:      r,
		config:      config,
	}, nil
}

func (a *HTTPServer) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:           a.config.ServerAddr,
		MaxHeaderBytes: 1 << 20,
		Handler:        a.router,
	}

	if a.config.DisableTLS {
		return process.StartStopServer(ctx, server, defaultShutdownTimeout)
	}

	var clientCAs *x509.CertPool
	if a.config.ClientRootCAs != "" {
		clientCAs = x509.NewCertPool()
		ok := clientCAs.AppendCertsFromPEM([]byte(a.config.ClientRootCAs))
		if !ok {
			return errors.New("could not append additional provided client certs")
		}
	} else {
		var err error
		clientCAs, err = x509.SystemCertPool()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	server.TLSConfig = tlsutil.DefaultTLSConfig()
	server.TLSConfig.ClientAuth = tls.VerifyClientCertIfGiven
	server.TLSConfig.ClientCAs = clientCAs

	return process.StartStopTLSServer(ctx, server, defaultShutdownTimeout, a.config.TLSCert, a.config.TLSKey)
}

func (a *HTTPServer) GetRouter() *chi.Mux {
	return a.router
}
