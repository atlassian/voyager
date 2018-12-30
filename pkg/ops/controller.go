package ops

import (
	"context"

	"github.com/atlassian/ctrl"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"github.com/atlassian/voyager/pkg/ops/util/zappers"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/cache"
)

type Controller struct {
	Logger       *zap.Logger
	ReadyForWork func()

	RouteInformer cache.SharedIndexInformer
	API           RouteAPI
}

// Run begins watching and syncing.
func (c *Controller) Run(ctx context.Context) {
	c.Logger.Info("Starting Ops Route controller")
	defer c.Logger.Info("Shutting down Route controller")

	c.ReadyForWork()
	<-ctx.Done()
}

func (c *Controller) Process(ctx *ctrl.ProcessContext) (retriable bool, err error) {
	route := ctx.Object.(*ops_v1.Route)

	conflict, retriable, err := c.process(route)
	if conflict {
		return false, nil
	}
	return retriable, err
}

func (c *Controller) process(route *ops_v1.Route) (conflictRet, retriableRet bool, e error) {
	retriable, provider, err := NewProvider(c.Logger, route)
	if err != nil {
		return false, retriable, errors.Wrapf(err, "failed to parse schema from provider %q", route.ObjectMeta.Name)
	}

	c.Logger.Info("Processed provider", zappers.Route(route))
	c.API.AddOrUpdateProvider(provider)

	return false, false, nil
}
