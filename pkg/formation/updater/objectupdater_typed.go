package updater

import (
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	orchClient "github.com/atlassian/voyager/pkg/orchestration/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func StateUpdater(existingObjectsIndexer cache.Indexer, specCheck updater.SpecCheck, client orchClient.Interface) updater.ObjectUpdater {
	return updater.ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: updater.ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.OrchestrationV1().States(ns).Create(obj.(*orch_v1.State))
				return runtime.Object(result), createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.OrchestrationV1().States(ns).Update(obj.(*orch_v1.State))
				return runtime.Object(result), createErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.OrchestrationV1().States(ns).Delete(name, options)
				return deleteErr
			},
		},
	}
}
