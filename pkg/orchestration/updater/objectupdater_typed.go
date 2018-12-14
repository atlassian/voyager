package k8s

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func BundleUpdater(existingObjectsIndexer cache.Indexer, specCheck updater.SpecCheck, client smithClientset.Interface) updater.ObjectUpdater {
	return updater.ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: updater.ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.SmithV1().Bundles(ns).Create(obj.(*smith_v1.Bundle))
				return runtime.Object(result), createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.SmithV1().Bundles(ns).Update(obj.(*smith_v1.Bundle))
				return runtime.Object(result), updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.SmithV1().Bundles(ns).Delete(name, options)
				return deleteErr
			},
		},
	}
}
