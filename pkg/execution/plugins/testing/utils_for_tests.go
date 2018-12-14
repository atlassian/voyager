package testing

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	envResourcePrefix = voyager.Domain + "/envResourcePrefix"
)

func ConstructSecretDependency(name, namespace string, secretData map[string][]byte) smith_plugin.Dependency {
	return smith_plugin.Dependency{
		Actual: ConstructSecret(name, namespace, secretData),
	}
}

func ConstructBindingDependency(bindingName, namespace, secretName, instanceRefName, clusterServiceClassExternalName string, secretData map[string][]byte) smith_plugin.Dependency {
	return smith_plugin.Dependency{
		Actual: ConstructBinding(bindingName, namespace, secretName, instanceRefName),

		Outputs: []runtime.Object{
			ConstructSecret(secretName, namespace, secretData),
		},

		Auxiliary: []runtime.Object{
			ConstructInstance(instanceRefName, namespace, clusterServiceClassExternalName),
		},
	}
}

func ConstructInstance(name, namespace, clusterServiceClassExternalName string) *sc_v1b1.ServiceInstance {
	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
			Kind:       "ServiceInstance",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				envResourcePrefix: clusterServiceClassExternalName,
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalName: clusterServiceClassExternalName,
			},
		},
	}
}

func ConstructBinding(bindingName, namespace, secretName, instanceRefName string) *sc_v1b1.ServiceBinding {
	return &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
			Kind:       "ServiceBinding",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			SecretName: secretName,
			ServiceInstanceRef: sc_v1b1.LocalObjectReference{
				Name: instanceRefName,
			},
		},
	}
}

func ConstructSecret(name, namespace string, secretData map[string][]byte) *core_v1.Secret {
	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: secretData,
	}
}
