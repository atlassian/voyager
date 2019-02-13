package trebuchet

import (
	"context"
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"github.com/atlassian/voyager/pkg/util/auth"
)

type ReleaseInterface interface {
	CreateRelease(ctx context.Context, user auth.User, release *trebuchet_v1.Release) (*trebuchet_v1.Release, error)
	GetLatestRelease(ctx context.Context) (*trebuchet_v1.Release, error)
	GetRelease(ctx context.Context) (*trebuchet_v1.Release, error)
}

type ReleaseGroupInterface interface {
	CreateReleaseGroup(ctx context.Context, user auth.User, release *trebuchet_v1.ReleaseGroup) (*trebuchet_v1.ReleaseGroup, error)
	GetLatestReleaseGroup(ctx context.Context) (*trebuchet_v1.ReleaseGroup, error)
	GetReleaseGroup(ctx context.Context) (*trebuchet_v1.ReleaseGroup, error)
	DeleteReleaseGroup(ctx context.Context, release *trebuchet_v1.ReleaseGroup) error
}
