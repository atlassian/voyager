package wiringutil

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConsumerProducerServiceBinding constructs a ServiceBinding to be consumed by a Voyager Resource consumer.
// ServiceInstance that is bound to is produced by Voyager Resource producer.
// This function should be used to generate ServiceBindings between two Voyager Resources.
//
// This function assumes that producer exposes a single bindable ServiceInstance. If it is not the case and
// there is a need to bind to multiple ServiceInstances produced by the same producer, construct names
// (using ConsumerProducerResourceNameWithPostfix() and ConsumerProducerMetaNameWithPostfix() functions) with
// different postfixes.
func ConsumerProducerServiceBinding(consumer, producer voyager.ResourceName, serviceInstanceRef smith_v1.Reference) smith_v1.Resource {
	bindingResourceName := ConsumerProducerResourceNameWithPostfix(consumer, producer, "binding")
	bindingMetaName := ConsumerProducerMetaName(consumer, producer)
	return ServiceBinding(bindingResourceName, bindingMetaName, serviceInstanceRef)
}

// ResourceInternalServiceBinding constructs a ServiceBinding that is both produced and consumed by an
// Orchestration resource resource. This function should be used to generate ServiceBindings that are internal to an
// Orchestration resource.
func ResourceInternalServiceBinding(resource voyager.ResourceName, serviceInstance smith_v1.ResourceName, postfix string) smith_v1.Resource {
	serviceInstanceRef := smith_v1.Reference{
		Name:     ReferenceName(serviceInstance, "metadata", "name"),
		Resource: serviceInstance,
		Path:     "metadata.name",
	}
	// ServiceBinding resource name must not clash with ServiceInstance resource name  (if same postfix is specified)
	// hence the modified postfix
	bindingResourceName := ResourceNameWithPostfix(resource, postfix+"-binding")
	// ServiceBinding is named just like ServiceInstance for convenience (if same postfix is specified)
	bindingMetaName := MetaNameWithPostfix(resource, postfix)
	return ServiceBinding(bindingResourceName, bindingMetaName, serviceInstanceRef)
}

func ServiceBinding(bindingResourceName smith_v1.ResourceName, bindingMetaName string, serviceInstanceRef smith_v1.Reference) smith_v1.Resource {
	return smith_v1.Resource{
		References: []smith_v1.Reference{
			serviceInstanceRef,
		},
		Name: bindingResourceName,
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceBinding{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceBinding",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: bindingMetaName,
				},
				Spec: sc_v1b1.ServiceBindingSpec{
					InstanceRef: sc_v1b1.LocalObjectReference{
						Name: serviceInstanceRef.Ref(),
					},
					SecretName: bindingMetaName,
				},
			},
		},
	}
}
