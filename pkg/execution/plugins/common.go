package plugins

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Copy paste out of CloudFormation controller, to avoid having a messy dependency
// (it's a single struct, and is the external interface...)
type CloudformationServiceInstancePayload struct {
	Template         string            `json:"cfTemplate,omitempty"`
	Tags             map[string]string `json:"cfTags,omitempty"`
	Environment      string            `json:"environment,omitempty"`
	Parameters       map[string]string `json:"cfParameters,omitempty"`
	TemplateName     string            `json:"cfTemplateName,omitempty"`
	FilterParameters bool              `json:"filterParameters,omitempty"`
	StackName        string            `json:"stackName,omitempty"`
}

func FindBindingSecret(binding *sc_v1b1.ServiceBinding, list []runtime.Object) *core_v1.Secret {
	return FindSecret(binding.Namespace, binding.Spec.SecretName, list)
}

func FindSecret(namespace, name string, list []runtime.Object) *core_v1.Secret {
	for _, obj := range list {
		secret, ok := obj.(*core_v1.Secret)
		if !ok {
			continue
		}
		if secret.Name == name && secret.Namespace == namespace {
			return secret
		}
	}
	return nil
}

func FindServiceInstance(binding *sc_v1b1.ServiceBinding, list []runtime.Object) *sc_v1b1.ServiceInstance {
	for _, obj := range list {
		serviceInstance, ok := obj.(*sc_v1b1.ServiceInstance)
		if ok && serviceInstance.Namespace == binding.Namespace && serviceInstance.Name == binding.Spec.ServiceInstanceRef.Name {
			return serviceInstance
		}
	}
	return nil
}
