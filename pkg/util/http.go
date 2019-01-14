package util

import (
	"net"
	"net/http"
	"time"

	"github.com/atlassian/voyager/pkg/util/tlsutil"
	"golang.org/x/net/http2"
)

const (
	// A timeout of 0 is bad if a server hangs (i.e. if it crashes), therefore ensure that we are using a timeout that
	// is greater than 0.
	defaultClientTimeout = 60 * time.Second

	// A timeout of 0 means we could wait forever. Note: the OS may impose it's own timeout.
	defaultDialTimeout = 5 * time.Second
)

func DefaultTransport() *http.Transport {
	transport := &http.Transport{

		// http.ProxyFromEnvironment looks at HTTP_PROXY, HTTPS_PROXY, and NO_PROXY.
		Proxy: http.ProxyFromEnvironment,

		// Specifies the Dial function for creating unencrypted TCP connections.
		DialContext: (&net.Dialer{
			Timeout:   defaultDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout: 3 * time.Second,
		TLSClientConfig:     tlsutil.DefaultTLSClientConfig(),
		MaxIdleConns:        50,
		IdleConnTimeout:     60 * time.Second,
	}
	if err := http2.ConfigureTransport(transport); err != nil {
		panic(err)
	}
	return transport
}

func HTTPClient() *http.Client {
	return &http.Client{
		Transport: DefaultTransport(),
		Timeout:   defaultClientTimeout,
	}
}
