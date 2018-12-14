package apiservice

import (
	"crypto/x509"
	"net/http"

	"github.com/atlassian/voyager/pkg/util/auth"
)

// appropriated from pkg/authentication/request/x509 in kubernetes-apiserver
// kubernetes-1.11.0 01459b68eb5fee2dcf5ca0e29df8bcac89ead47b
// The changes made here include:
// * renaming Verifier to X509Authenticator.
// * changing the signature of AuthenticateRequest to return our own internal
//   auth.AggregatorUserInfo type.

/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

type X509Authenticator struct {
	opts x509.VerifyOptions
	auth Authenticator
}

// NewVerifier verifies a client cert on the request, then delegating to the wrapped auth
func NewX509Authenticator(opts x509.VerifyOptions, auth Authenticator) *X509Authenticator {
	return &X509Authenticator{opts, auth}
}

func (a *X509Authenticator) AuthenticateRequest(req *http.Request) (auth.AggregatorUserInfo, bool, error) {
	if req.TLS == nil || len(req.TLS.PeerCertificates) == 0 {
		return auth.AggregatorUserInfo{}, false, nil
	}

	// Use intermediates, if provided
	optsCopy := a.opts
	if optsCopy.Intermediates == nil && len(req.TLS.PeerCertificates) > 1 {
		optsCopy.Intermediates = x509.NewCertPool()
		for _, intermediate := range req.TLS.PeerCertificates[1:] {
			optsCopy.Intermediates.AddCert(intermediate)
		}
	}

	if _, err := req.TLS.PeerCertificates[0].Verify(optsCopy); err != nil {
		return auth.AggregatorUserInfo{}, false, err
	}
	return a.auth.AuthenticateRequest(req)
}

// DefaultVerifyOptions returns VerifyOptions that use the system root certificates, current time,
// and requires certificates to be valid for client auth (x509.ExtKeyUsageClientAuth)
func DefaultVerifyOptions() x509.VerifyOptions {
	return x509.VerifyOptions{
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
}
