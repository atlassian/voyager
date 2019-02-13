package trebuchet

import (
	"context"
	"net/http"
	"net/url"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"go.uber.org/zap"

	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
)

const (
	v1ReleaseGroupPath = "/api/v1/releasesgroups"
)

// Client is a minimalistic Service Central client for our needs
// See the outdated API spec there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/docs/api.raml
// Or the actual API code (Python) there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/service_central/api.py
type ReleaseGroupClient struct {
	logger     *zap.Logger
	httpClient *http.Client
	asap       pkiutil.ASAP
	rm         *restclient.RequestMutator
}

func NewReleaseGroupClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *ReleaseGroupClient {
	rm := restclient.NewRequestMutator(
		restclient.BaseURL(baseURL.String()),
	)
	return &ReleaseGroupClient{
		logger:     logger,
		httpClient: httpClient,
		asap:       asap,
		rm:         rm,
	}
}

func (c *ReleaseGroupClient) CreateOrUpdateReleaseGroup(ctx context.Context, service string, release *trebuchet_v1.ReleaseGroup) (*trebuchet_v1.ReleaseGroup, error) {

}

func (c *ReleaseGroupClient) GetLatestReleaseGroup(ctx context.Context, service string) (*trebuchet_v1.ReleaseGroup, error) {

}

func (c *ReleaseGroupClient) GetReleaseGroup(ctx context.Context, service string) (*trebuchet_v1.ReleaseGroup, error) {

}

func (c *ReleaseGroupClient) DeleteReleaseGroup(ctx context.Context, service string, release *trebuchet_v1.ReleaseGroup) error {

}
