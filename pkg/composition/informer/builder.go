package informer

import (
	"time"

	"github.com/atlassian/ctrl"
	compClient "github.com/atlassian/voyager/pkg/composition/client"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func CompositionInformer(config *ctrl.Config, cctx *ctrl.Context, sdClient compClient.Interface, gvk schema.GroupVersionKind, f func(compClient.Interface, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(sdClient, config.ResyncPeriod)
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
