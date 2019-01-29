package svccatadmission

import (
	"context"

	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
)

type serviceCentralClient interface {
	ListServices(ctx context.Context, user auth.OptionalUser, search string) ([]servicecentral.ServiceData, error)
}

type microsServerClient interface {
	GetAlias(ctx context.Context, domainName string) (*microsserver.AliasInfo, error)
}
