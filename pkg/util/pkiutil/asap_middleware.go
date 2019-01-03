package pkiutil

import (
	"fmt"
	"net/http"

	"bitbucket.org/atlassianlabs/restclient"
)

// SSAM API Documentation - https://ssam.office.atlassian.com/api/
func AuthenticateWithASAP(asap ASAP, audience, subject string) restclient.RequestMutation {
	return func(request *http.Request) (*http.Request, error) {
		token, err := asap.GenerateToken(audience, subject)
		if err != nil {
			return nil, err
		}
		return restclient.Header("Authorization", fmt.Sprintf("Bearer %s", string(token)))(request)
	}
}
