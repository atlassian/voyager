package informer

import (
	"time"

	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	orchClientset "github.com/atlassian/voyager/pkg/orchestration/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func StateInformer(client orchClientset.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	states := client.OrchestrationV1().States(namespace)
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return states.List(options)
			},
			WatchFunc: states.Watch,
		},
		&orch_v1.State{},
		resyncPeriod,
		cache.Indexers{})
}
