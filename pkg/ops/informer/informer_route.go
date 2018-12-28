package informer

import (
	"time"

	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	ops_v1client "github.com/atlassian/voyager/pkg/ops/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func RouteInformer(client ops_v1client.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	routes := client.OpsV1().Routes(namespace)
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return routes.List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return routes.Watch(options)
			},
		},
		&ops_v1.Route{},
		resyncPeriod,
		cache.Indexers{})
}
