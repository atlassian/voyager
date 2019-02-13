package trebuchet

import (
	"context"
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
)

type ReleaseInterface interface {
	CreateRelease(ctx context.Context, service string, release *trebuchet_v1.Release) (*trebuchet_v1.Release, error)
	GetLatestRelease(ctx context.Context, service string) (*trebuchet_v1.Release, error)
	GetRelease(ctx context.Context, service string, uuid string) (*trebuchet_v1.Release, error)
}

type ReleaseGroupInterface interface {
	CreateOrUpdateReleaseGroup(ctx context.Context, service string, release *trebuchet_v1.ReleaseGroup) (*trebuchet_v1.ReleaseGroup, error)
	GetLatestReleaseGroup(ctx context.Context, service string) (*trebuchet_v1.ReleaseGroup, error)
	GetReleaseGroup(ctx context.Context, service string) (*trebuchet_v1.ReleaseGroup, error)
	DeleteReleaseGroup(ctx context.Context, service string, release *trebuchet_v1.ReleaseGroup) error
}
