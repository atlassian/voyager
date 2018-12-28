package informer

import (
	"time"

	"github.com/atlassian/ctrl"
	opsClient "github.com/atlassian/voyager/pkg/ops/client"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func OpsInformer(config *ctrl.Config, cctx *ctrl.Context, opsClient opsClient.Interface, gvk schema.GroupVersionKind, f func(opsClient.Interface, string, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(opsClient, config.Namespace, config.ResyncPeriod)
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
