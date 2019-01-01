package creator

import (
	"context"
	"time"

	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/creator/luigi"
	"github.com/atlassian/voyager/pkg/creator/ssam"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
)

type ServiceCentralStoreInterface interface {
	FindOrCreateService(ctx context.Context, user auth.User, service *creator_v1.Service) (*creator_v1.Service, error)
	GetService(ctx context.Context, user auth.OptionalUser, name servicecentral.ServiceName) (*creator_v1.Service, error)
	ListServices(ctx context.Context, user auth.OptionalUser) ([]creator_v1.Service, error)
	ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]creator_v1.Service, error)
	PatchService(ctx context.Context, user auth.User, service *creator_v1.Service) error
	DeleteService(ctx context.Context, user auth.User, name servicecentral.ServiceName) error
}

type PagerDutyClientInterface interface {
	FindOrCreate(serviceName voyager.ServiceName, user auth.User, email string) (creator_v1.PagerDutyMetadata, error)
	Delete(serviceName voyager.ServiceName) error
}

type LuigiClientInterface interface {
	FindOrCreateService(ctx context.Context, meta *luigi.ServiceMetadata) (*luigi.BasicServiceData, error)
	DeleteService(ctx context.Context, loggingID string) error
}

type SSAMClientInterface interface {
	GetExpectedServiceContainerName(ctx context.Context, metadata *ssam.ServiceMetadata) string
	CreateService(ctx context.Context, metadata *ssam.ServiceMetadata) (string, ssam.AccessLevels, error)
	DeleteService(ctx context.Context, metadata *ssam.ServiceMetadata) error
}
