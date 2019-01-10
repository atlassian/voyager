package tlsutil

import (
	"crypto/tls"
)

// DefaultTLSConfig aims to implement secure defaults for TLS and implement the following policies:
//  * Standard - Server Side Transport Layer Security (TLS) for new and existing services
//  * Guideline - Modern Server Side Transport Layer Security (TLS)
func DefaultTLSConfig() *tls.Config {
	return &tls.Config{

		// Everything prior to TLS1.2 has vulnerabilities.
		MinVersion: tls.VersionTLS12,

		// Enable HTTP2
		NextProtos: []string{"h2", "http/1.1"},

		// TLS cipher suite preference must be dictated by the server
		PreferServerCipherSuites: true,

		// List of cipher suites comes from the standard.
		// IANA names were mapped from testssl.sh and ciphersuite.info
		CipherSuites: []uint16{
			// These aren't supported in Go (yet), but are allowed by our standard.
			// tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384,          // ECDHE-ECDSA-AES256-SHA384
			// tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384,            // ECDHE-RSA-AES256-SHA384
			// tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,    // ECDHE-ECDSA-CHACHA20-POLY1305
			// tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,    // ECDHE-RSA-CHACHA20-POLY1305

			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, // ECDHE-ECDSA-AES128-SHA256
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,    // ECDHE-ECDSA-AES128-SHA
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,    // ECDHE-ECDSA-AES256-SHA
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,   // ECDHE-RSA-AES128-SHA256
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // ECDHE-ECDSA-AES128-GCM-SHA256
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // ECDHE-ECDSA-AES256-GCM-SHA384
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,   // ECDHE-RSA-AES128-GCM-SHA256
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // ECDHE-RSA-AES256-GCM-SHA384
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,      // ECDHE-RSA-AES128-SHA
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,      // ECDHE-RSA-AES256-SHA
		},
	}
}

func DefaultTLSClientConfig() *tls.Config {
	return &tls.Config{
		// Everything prior to TLS1.2 has vulnerabilities.
		MinVersion: tls.VersionTLS12,

		// Enable HTTP2
		NextProtos: []string{"h2", "http/1.1"},
	}
}
