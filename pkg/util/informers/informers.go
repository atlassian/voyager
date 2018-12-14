package informers

import (
	"time"

	"github.com/atlassian/ctrl"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

func SmithInformer(config *ctrl.Config, cctx *ctrl.Context, smithClient smithClientset.Interface, gvk schema.GroupVersionKind, f func(smithClientset.Interface, string, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(smithClient, config.Namespace, config.ResyncPeriod)
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func SvcCatInformer(config *ctrl.Config, cctx *ctrl.Context, scClient scClientset.Interface, gvk schema.GroupVersionKind, f func(scClientset.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := cctx.Informers[gvk]
	if inf == nil {
		inf = f(scClient, config.Namespace, config.ResyncPeriod, cache.Indexers{})
		err := cctx.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
