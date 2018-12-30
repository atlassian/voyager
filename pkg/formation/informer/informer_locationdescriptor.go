package informer

import (
	"time"

	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	"github.com/atlassian/voyager/pkg/formation/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func LocationDescriptorInformer(client client.Interface, namespace string, resync time.Duration) cache.SharedIndexInformer {
	lds := client.FormationV1().LocationDescriptors(namespace)
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return lds.List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return lds.Watch(options)
			},
		},
		&form_v1.LocationDescriptor{},
		resync,
		cache.Indexers{},
	)
}
