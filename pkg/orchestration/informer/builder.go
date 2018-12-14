package informer

import (
	"time"

	"github.com/atlassian/ctrl"
	orchClient "github.com/atlassian/voyager/pkg/orchestration/client"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func OrchestrationInformer(config *ctrl.Config, cctx *ctrl.Context, orchClient orchClient.Interface, gvk schema.GroupVersionKind, f func(orchClient.Interface, string, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(orchClient, config.Namespace, config.ResyncPeriod)
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
