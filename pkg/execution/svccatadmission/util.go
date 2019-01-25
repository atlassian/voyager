package svccatadmission

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/pkg/errors"
)

func getServiceNameFromNamespace(namespace string) voyager.ServiceName {
	// This is a nasty hack for now so that the admission controller
	// doesn't need to talk to the cluster. It may break if we ever change how
	// this works (but we're unlikely to)... the good news is, it should break in a clean way
	// (the service probably won't exist, so we will fail to do anything here).
	// DO NOT replicate this approach in other parts of the codebase without discussion.
	return voyager.ServiceName(strings.Split(namespace, "--")[0])
}

func getServiceData(ctx context.Context, scClient serviceCentralClient, serviceName voyager.ServiceName) (*servicecentral.ServiceData, error) {
	search := fmt.Sprintf("service_name='%s'", serviceName)
	listData, err := scClient.ListServices(ctx, auth.NoUser(), search)

	if err != nil {
		return nil, errors.Wrapf(err, "error looking up service %q", serviceName)
	}

	for _, serviceData := range listData {
		if voyager.ServiceName(serviceData.ServiceName) == serviceName {
			return &serviceData, nil
		}
	}

	return nil, nil
}
