package rps

import (
	"context"
	"sync"

	"github.com/atlassian/voyager"
	"go.uber.org/zap"
)

type ServiceMetadata struct {
	Name         string
	BusinessUnit string
	Owner        string
}

// Cache caches its single 'list' operation indefinitely.
// The theory is that we only look up instances that should be migrated,
// and since we 'restart' svccatadmission frequently we should never be too out of date.
// Also, we're unlikely to ever care about resources created in the last few days,
// because there won't be anything meaningful to migrate (i.e. users won't want to
// move them), and the failure mode is simply a rejection.
// We could have a smarter cache that remembers the time (but, more complexity we don't
// need?) or retry if unknown UUID (but most likely scenario is a _bad_ UUID, which
// could lead to the user hammering the Dynamo table and exhausting the RCU, which
// is what we want to avoid).
type Cache struct {
	logger    *zap.Logger
	rpsClient Client

	serviceNamesByInstanceID map[string]voyager.ServiceName
	// Look, we don't strictly need this, because it's a RO map after creation, but
	// I guess it prevents an unlikely deluge of REST requests in the initial stages
	// (or indefinitely if we're 500ing)...
	serviceNamesLock sync.Mutex
}

func NewRPSCache(logger *zap.Logger, client Client) *Cache {
	return &Cache{
		logger:    logger,
		rpsClient: client,
	}
}

func (r *Cache) updateCache(ctx context.Context) error {
	if r.serviceNamesByInstanceID != nil {
		// aha, someone else must have updated me!
		return nil
	}

	osbResources, err := r.rpsClient.ListOSBResources(ctx)
	if err != nil {
		return err
	}

	serviceNamesByInstanceID := make(map[string]voyager.ServiceName, len(osbResources))
	for _, osbResource := range osbResources {
		serviceNamesByInstanceID[osbResource.InstanceID] = osbResource.ServiceID
	}
	r.serviceNamesByInstanceID = serviceNamesByInstanceID
	return nil
}

func (r *Cache) GetServiceFor(ctx context.Context, instanceID string) (voyager.ServiceName, error) {
	r.serviceNamesLock.Lock()
	defer r.serviceNamesLock.Unlock()

	if r.serviceNamesByInstanceID == nil {
		if err := r.updateCache(ctx); err != nil {
			return "", err
		}
	}

	return r.serviceNamesByInstanceID[instanceID], nil
}
