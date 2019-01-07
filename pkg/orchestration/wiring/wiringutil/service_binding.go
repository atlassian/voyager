package wiringutil

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
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
// DEPRECATED: use ConsumerProducerServiceBindingV2 instead
func ConsumerProducerServiceBinding(consumer, producer voyager.ResourceName, producerServiceInstance smith_v1.ResourceName,
	exposed bool) wiringplugin.WiredSmithResource {

	serviceInstanceRef := smith_v1.Reference{
		Name:     ReferenceName(producerServiceInstance, "metadata", "name"),
		Resource: producerServiceInstance,
		Path:     "metadata.name",
	}
	bindingResourceName := ConsumerProducerResourceNameWithPostfix(consumer, producer, "binding")
	bindingMetaName := ConsumerProducerMetaName(consumer, producer)
	return ServiceBinding(bindingResourceName, bindingMetaName, serviceInstanceRef, exposed)
}

// TODO(kopper): Remove V2 suffix
func ConsumerProducerServiceBindingV2(consumer, producer voyager.ResourceName, resourceReference wiringplugin.ProtoReference,
	exposed bool) wiringplugin.WiredSmithResource {
	bindingResourceName := ConsumerProducerResourceNameWithPostfix(consumer, producer, "binding")
	bindingMetaName := ConsumerProducerMetaName(consumer, producer)
	referenceName := ReferenceName(resourceReference.Resource, "metadata", "name")
	serviceInstanceRef := resourceReference.ToReference(referenceName)
	return ServiceBinding(bindingResourceName, bindingMetaName, serviceInstanceRef, exposed)
}

// ResourceInternalServiceBinding constructs a ServiceBinding that is both produced and consumed by an
// Orchestration resource resource. This function should be used to generate ServiceBindings that are internal to an
// Orchestration resource.
func ResourceInternalServiceBinding(resource voyager.ResourceName, serviceInstance smith_v1.ResourceName,
	postfix string, exposed bool) wiringplugin.WiredSmithResource {

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
	return ServiceBinding(bindingResourceName, bindingMetaName, serviceInstanceRef, exposed)
}

func ServiceBinding(bindingResourceName smith_v1.ResourceName, bindingMetaName string,
	serviceInstanceRef smith_v1.Reference, exposed bool) wiringplugin.WiredSmithResource {

	return wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
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
						ServiceInstanceRef: sc_v1b1.LocalObjectReference{
							Name: serviceInstanceRef.Ref(),
						},
						SecretName: bindingMetaName,
					},
				},
			},
		},
		Exposed: exposed,
	}
}
