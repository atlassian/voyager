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
	v1ReleasesPath = "/api/v1/releases" // TODO: make this url correct

	asapAudience = "deployinator-trebuchet"
)

// Client is a minimalistic Service Central client for our needs
// See the outdated API spec there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/docs/api.raml
// Or the actual API code (Python) there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/service_central/api.py
type ReleaseClient struct {
	logger     *zap.Logger
	httpClient *http.Client
	asap       pkiutil.ASAP
	rm         *restclient.RequestMutator
}

// TODO: make sure config file contains this info
func NewReleaseClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *ReleaseClient {
	rm := restclient.NewRequestMutator(
		restclient.BaseURL(baseURL.String()),
	)
	return &ReleaseClient{
		logger:     logger,
		httpClient: httpClient,
		asap:       asap,
		rm:         rm,
	}
}

// Create a new release. A new UUID will be returned
// return codes:
// - 201: Service created
// - 400: Bad request
// - 409: The service UUID or service name already exists
// - 500: Internal server error
func (c *ReleaseClient) CreateRelease(ctx context.Context, service string, release *trebuchet_v1.Release) (*trebuchet_v1.Release, error) {

}

// Patch an existing service with the attached specification
// return codes:
// - 200: The service was successfully updated
func (c *ReleaseClient) GetLatestRelease(ctx context.Context, service string) (*trebuchet_v1.Release, error) {

}


func (c *ReleaseClient) GetRelease(ctx context.Context, service string, uuid string) (*trebuchet_v1.Release, error) {

}
