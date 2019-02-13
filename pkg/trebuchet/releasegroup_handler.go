package trebuchet

import (
	"context"
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"github.com/atlassian/voyager/pkg/util"
)

type ReleaseGroupHandler struct {
	trebuchet ReleaseGroupInterface
}

func NewReleaseGroupHandler(config *ExtraConfig) (*ReleaseGroupHandler, error) {

	httpClient := util.HTTPClient()
	releaseGroupClient := NewReleaseGroupClient(config.Logger, httpClient, config.ASAPClientConfig, config.DeployinatorURL)

	return &ReleaseGroupHandler{
		trebuchet: releaseGroupClient,
	}, nil
}


func (h *ReleaseGroupHandler) CreateOrUpdateReleaseGroup(ctx context.Context, service string, releaseGroup *trebuchet_v1.ReleaseGroup) (*trebuchet_v1.ReleaseGroup, error) {
	return h.trebuchet.CreateOrUpdateReleaseGroup(ctx, service, releaseGroup)
}

func (h *ReleaseGroupHandler) GetReleaseGroups(ctx context.Context, service string) (*trebuchet_v1.ReleaseGroup, error) {
	return h.trebuchet.GetReleaseGroup(ctx, service)
}

func (h *ReleaseGroupHandler) DeleteReleaseGroup(ctx context.Context, service string, releaseGroup *trebuchet_v1.ReleaseGroup) error {
	return h.trebuchet.DeleteReleaseGroup(ctx, service, releaseGroup)
}
