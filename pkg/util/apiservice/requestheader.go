package apiservice

import (
	"crypto/x509"
	"net/http"
	"strings"

	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/pkg/errors"
	"k8s.io/client-go/util/cert"
)

const (
	CertificateBlockType = "CERTIFICATE"
)

// appropriated from pkg/authentication/request/headerrequest in kubernetes-apiserver
// kubernetes-1.11.0 01459b68eb5fee2dcf5ca0e29df8bcac89ead47b
// The changes made here include:
// * renaming requestHeaderAuthRequestHandler to RequestHeaderAuthenticator.
// * removing the `proxyClientNames` argument because we don't usually need it.
// * changing the signature of AuthenticateRequest to return our own internal
//   auth.AggregatorUserInfo type.
// * creating a new X509RequestHeaderAuthenticator to represent the `NewSecure`
//   Verifier for additional type safety.

type RequestHeaderAuthenticator struct {
	nameHeaders         []string
	groupHeaders        []string
	extraHeaderPrefixes []string
}

type X509RequestHeaderAuthenticator struct {
	delegate *X509Authenticator
	wrapped  *RequestHeaderAuthenticator
}

func NewRequestHeaderAuthenticator(nameHeaders []string, groupHeaders []string, extraHeaderPrefixes []string) (*RequestHeaderAuthenticator, error) {
	trimmedNameHeaders, err := trimHeaders(nameHeaders...)
	if err != nil {
		return nil, err
	}
	trimmedGroupHeaders, err := trimHeaders(groupHeaders...)
	if err != nil {
		return nil, err
	}
	trimmedExtraHeaderPrefixes, err := trimHeaders(extraHeaderPrefixes...)
	if err != nil {
		return nil, err
	}

	return &RequestHeaderAuthenticator{
		nameHeaders:         trimmedNameHeaders,
		groupHeaders:        trimmedGroupHeaders,
		extraHeaderPrefixes: trimmedExtraHeaderPrefixes,
	}, nil
}

func NewX509RequestHeaderAuthenticator(clientCA string, nameHeaders []string, groupHeaders []string, extraHeaderPrefixes []string) (*X509RequestHeaderAuthenticator, error) {
	headerAuthenticator, err := NewRequestHeaderAuthenticator(nameHeaders, groupHeaders, extraHeaderPrefixes)
	if err != nil {
		return nil, err
	}

	if len(clientCA) == 0 {
		return nil, errors.New("missing clientCA file")
	}

	// Wrap with an x509 verifier
	caData := []byte(clientCA)
	opts := DefaultVerifyOptions()
	opts.Roots = x509.NewCertPool()
	certs, err := cert.ParseCertsPEM(caData)
	if err != nil {
		return nil, errors.Errorf("error loading certs from  %s: %v", clientCA, err)
	}
	for _, cert := range certs {
		opts.Roots.AddCert(cert)
	}

	x509WrappedAuthenticator := NewX509Authenticator(opts, headerAuthenticator)

	return &X509RequestHeaderAuthenticator{
		delegate: x509WrappedAuthenticator,
		wrapped:  headerAuthenticator,
	}, nil
}

func (r *X509RequestHeaderAuthenticator) AuthenticateRequest(req *http.Request) (auth.AggregatorUserInfo, bool, error) {
	return r.delegate.AuthenticateRequest(req)
}

func (r *RequestHeaderAuthenticator) AuthenticateRequest(req *http.Request) (auth.AggregatorUserInfo, bool, error) {
	name := headerValue(req.Header, r.nameHeaders)
	if len(name) == 0 {
		return auth.AggregatorUserInfo{}, false, nil
	}
	groups := allHeaderValues(req.Header, r.groupHeaders)
	extra := newExtra(req.Header, r.extraHeaderPrefixes)

	// clear headers used for authentication
	for _, headerName := range r.nameHeaders {
		req.Header.Del(headerName)
	}
	for _, headerName := range r.groupHeaders {
		req.Header.Del(headerName)
	}
	for k := range extra {
		for _, prefix := range r.extraHeaderPrefixes {
			req.Header.Del(prefix + k)
		}
	}

	return auth.AggregatorUserInfo{
		User:   name,
		Groups: groups,
		Extra:  extra,
	}, true, nil
}

func trimHeaders(headerNames ...string) ([]string, error) {
	ret := []string{}
	for _, headerName := range headerNames {
		trimmedHeader := strings.TrimSpace(headerName)
		if len(trimmedHeader) == 0 {
			return nil, errors.Errorf("empty header %q", headerName)
		}
		ret = append(ret, trimmedHeader)
	}

	return ret, nil
}

func headerValue(h http.Header, headerNames []string) string {
	for _, headerName := range headerNames {
		headerValue := h.Get(headerName)
		if len(headerValue) > 0 {
			return headerValue
		}
	}
	return ""
}

func allHeaderValues(h http.Header, headerNames []string) []string {
	ret := []string{}
	for _, headerName := range headerNames {
		headerKey := http.CanonicalHeaderKey(headerName)
		values, ok := h[headerKey]
		if !ok {
			continue
		}

		for _, headerValue := range values {
			if len(headerValue) > 0 {
				ret = append(ret, headerValue)
			}
		}
	}
	return ret
}

func newExtra(h http.Header, headerPrefixes []string) map[string][]string {
	ret := map[string][]string{}

	// we have to iterate over prefixes first in order to have proper ordering inside the value slices
	for _, prefix := range headerPrefixes {
		for headerName, vv := range h {
			if !strings.HasPrefix(strings.ToLower(headerName), strings.ToLower(prefix)) {
				continue
			}

			extraKey := strings.ToLower(headerName[len(prefix):])
			ret[extraKey] = append(ret[extraKey], vv...)
		}
	}

	return ret
}
