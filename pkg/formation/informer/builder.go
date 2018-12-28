package informer

import (
	"time"

	"github.com/atlassian/ctrl"
	formClient "github.com/atlassian/voyager/pkg/formation/client"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func FormationInformer(config *ctrl.Config, cctx *ctrl.Context, formClient formClient.Interface, gvk schema.GroupVersionKind, f func(formClient.Interface, string, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {

	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(formClient, config.Namespace, config.ResyncPeriod)
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil

}
