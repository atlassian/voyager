package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/atlassian/ctrl/process"
	"github.com/atlassian/voyager/pkg/options"
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

	server.TLSConfig = &tls.Config{
		// Can't use SSLv3 because of POODLE and BEAST
		// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
		// Can't use TLSv1.1 because of RC4 cipher usage
		MinVersion: tls.VersionTLS12,
		// enable HTTP2 for go's 1.7 HTTP Server
		NextProtos: []string{"h2", "http/1.1"},
		// aggregator posts client cert
		ClientAuth: tls.VerifyClientCertIfGiven,
		// set client cas
		ClientCAs: clientCAs,
	}

	return process.StartStopTLSServer(ctx, server, defaultShutdownTimeout, a.config.TLSCert, a.config.TLSKey)
}

func (a *HTTPServer) GetRouter() *chi.Mux {
	return a.router
}
