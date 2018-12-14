package options

import (
	"github.com/pkg/errors"
)

// ServerConfig contains options for running HTTP servers
type ServerConfig struct {

	// TLSCert is the location of the tls cert file
	TLSCert string `json:"tlsCert"`

	// TLSKey is the location of the tls key file
	TLSKey string `json:"tlsKey"`

	// ClientRootCAs is the concatenated list of CAs for client cert validation
	ClientRootCAs string `json:"clientRootCAs"`

	// ServerAddr is the Address to serve on. Defaults to port 443
	ServerAddr string `json:"serverAddr"`

	// DisableTLS indicates whether to disable TLS;
	// should be used only for local testing and integration tests
	DisableTLS bool `json:"disableTls"`
}

func (conf *ServerConfig) DefaultAndValidate() []error {
	var allErrors []error
	if conf.TLSCert == "" {
		allErrors = append(allErrors, errors.New("missing TLS Cert"))
	}
	if conf.TLSKey == "" {
		allErrors = append(allErrors, errors.New("missing TLS Key"))
	}

	if conf.ServerAddr == "" {
		conf.ServerAddr = ":443"
	}

	return allErrors
}
