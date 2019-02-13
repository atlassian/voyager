package trebuchet

import (
	"net/http"
	"net/url"

	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"go.uber.org/zap"
)

type ExtraConfig struct {
	Logger           *zap.Logger
	ASAPClientConfig pkiutil.ASAP
	HTTPClient       *http.Client
	DeployinatorURL  *url.URL
}
