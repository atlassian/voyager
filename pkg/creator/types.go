package creator

import (
	"net/http"
	"net/url"

	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/pkiutil"
	"go.uber.org/zap"
)

type ExtraConfig struct {
	Logger            *zap.Logger
	SSAMURL           *url.URL
	ServiceCentralURL *url.URL
	LuigiURL          *url.URL
	PagerDuty         pagerduty.ClientConfig
	ASAPClientConfig  pkiutil.ASAP
	HTTPClient        *http.Client
}
