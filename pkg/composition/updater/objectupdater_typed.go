package k8s

import (
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	formClient "github.com/atlassian/voyager/pkg/formation/client"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func LocationDescriptorUpdater(existingObjectsIndexer cache.Indexer, specCheck updater.SpecCheck, client formClient.Interface) updater.ObjectUpdater {
	return updater.ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: updater.ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, err := client.FormationV1().LocationDescriptors(ns).Create(obj.(*form_v1.LocationDescriptor))
				return runtime.Object(result), err
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, err := client.FormationV1().LocationDescriptors(ns).Update(obj.(*form_v1.LocationDescriptor))
				return runtime.Object(result), err
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.FormationV1().LocationDescriptors(ns).Delete(name, options)
				return deleteErr
			},
		},
	}
}
