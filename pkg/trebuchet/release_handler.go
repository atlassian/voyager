package trebuchet

import (
	"context"
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"github.com/atlassian/voyager/pkg/util"
)

type ReleaseHandler struct {
	trebuchet ReleaseInterface
}

func NewReleaseHandler(config *ExtraConfig) (*ReleaseHandler, error) {
	httpClient := util.HTTPClient()
	releaseClient := NewReleaseClient(config.Logger, httpClient, config.ASAPClientConfig, config.DeployinatorURL)

	return &ReleaseHandler{
		trebuchet: releaseClient,
	}, nil
}


func (h *ReleaseHandler) CreateRelease(ctx context.Context, service string, release *trebuchet_v1.Release) (*trebuchet_v1.Release, error) {
	return h.trebuchet.CreateRelease(ctx, service, release)
}

func (h *ReleaseHandler) GetRelease(ctx context.Context, service string, uuid string) (*trebuchet_v1.Release, error) {
	return h.trebuchet.GetRelease(ctx, service, uuid)
}

func (h *ReleaseHandler) GetLatestRelease(ctx context.Context, service string) (*trebuchet_v1.Release, error) {
	return h.trebuchet.GetLatestRelease(ctx, service)
}
