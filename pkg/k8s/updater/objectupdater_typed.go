package updater

import (
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func RoleBindingUpdater(existingObjectsIndexer cache.Indexer, specCheck SpecCheck, client kubernetes.Interface) ObjectUpdater {
	return ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.RbacV1().RoleBindings(ns).Create(obj.(*rbac_v1.RoleBinding))
				return result, createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.RbacV1().RoleBindings(ns).Update(obj.(*rbac_v1.RoleBinding))
				return result, updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.RbacV1().RoleBindings(ns).Delete(name, options)
				return deleteErr
			},
		},
	}
}

func ConfigMapUpdater(existingObjectsIndexer cache.Indexer, specCheck SpecCheck, client kubernetes.Interface) ObjectUpdater {
	return ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.CoreV1().ConfigMaps(ns).Create(obj.(*core_v1.ConfigMap))
				return result, createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.CoreV1().ConfigMaps(ns).Update(obj.(*core_v1.ConfigMap))
				return result, updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.CoreV1().ConfigMaps(ns).Delete(name, options)
				return deleteErr
			},
		},
	}
}

func NamespaceUpdater(existingObjectsIndexer cache.Indexer, specCheck SpecCheck, client kubernetes.Interface) ObjectUpdater {
	return ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.CoreV1().Namespaces().Create(obj.(*core_v1.Namespace))
				return result, createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.CoreV1().Namespaces().Update(obj.(*core_v1.Namespace))
				return result, updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.CoreV1().Namespaces().Delete(name, options)
				return deleteErr
			},
		},
	}
}

func ClusterRoleUpdater(existingObjectsIndexer cache.Indexer, specCheck SpecCheck, client kubernetes.Interface) ObjectUpdater {
	return ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.RbacV1().ClusterRoles().Create(obj.(*rbac_v1.ClusterRole))
				return result, createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.RbacV1().ClusterRoles().Update(obj.(*rbac_v1.ClusterRole))
				return result, updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.RbacV1().ClusterRoles().Delete(name, options)
				return deleteErr
			},
		},
	}
}

func ClusterRoleBindingUpdater(existingObjectsIndexer cache.Indexer, specCheck SpecCheck, client kubernetes.Interface) ObjectUpdater {
	return ObjectUpdater{
		SpecCheck:              specCheck,
		ExistingObjectsIndexer: existingObjectsIndexer,
		Client: ClientAdapter{
			CreateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, createErr := client.RbacV1().ClusterRoleBindings().Create(obj.(*rbac_v1.ClusterRoleBinding))
				return result, createErr
			},
			UpdateMethod: func(ns string, obj runtime.Object) (runtime.Object, error) {
				result, updateErr := client.RbacV1().ClusterRoleBindings().Update(obj.(*rbac_v1.ClusterRoleBinding))
				return result, updateErr
			},
			DeleteMethod: func(ns string, name string, options *meta_v1.DeleteOptions) error {
				deleteErr := client.RbacV1().ClusterRoleBindings().Delete(name, options)
				return deleteErr
			},
		},
	}
}
