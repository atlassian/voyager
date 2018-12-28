package informer

import (
	"time"

	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/composition/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func ServiceDescriptorInformer(client client.Interface, resync time.Duration) cache.SharedIndexInformer {
	sd := client.CompositionV1().ServiceDescriptors()
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return sd.List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return sd.Watch(options)
			},
		},
		&comp_v1.ServiceDescriptor{},
		resync,
		cache.Indexers{},
	)
}
