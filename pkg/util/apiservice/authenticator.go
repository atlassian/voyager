package apiservice

import (
	"net/http"

	"github.com/atlassian/voyager/pkg/util/auth"
)

type Authenticator interface {
	AuthenticateRequest(req *http.Request) (auth.AggregatorUserInfo, bool, error)
}
