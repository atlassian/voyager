package svccatentangler

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type OptionalShapeFunc func(*orch_v1.StateResource, *smith_v1.Resource, *wiringplugin.WiringContext) ([]wiringplugin.Shape, error)

func NoOptionalShapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, error) {
	return nil, nil
}

type SvcCatEntangler struct {
	ClusterServiceClassExternalID servicecatalog.ClassExternalID
	ClusterServicePlanExternalID  servicecatalog.PlanExternalID

	InstanceSpec func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]byte, error)
	ObjectMeta   func(*orch_v1.StateResource, *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error)
	References   func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]smith_v1.Reference, error)

	ResourceType voyager.ResourceType

	// optional shapes by resource type
	OptionalShapes OptionalShapeFunc
}

type partialSpec struct {
	InstanceID string `json:"instanceId"`
}

// Gets instanceId from resource's spec if present or "" otherwise
// If the spec is empty or does not contain instance ID, then this returns the empty string.
func InstanceID(resourceSpec *runtime.RawExtension) (string, error) {
	if resourceSpec == nil {
		return "", nil
	}

	spec := partialSpec{}
	err := json.Unmarshal(resourceSpec.Raw, &spec)
	if err != nil {
		return "", errors.Wrap(err, "unmarshalling StateResource failed")
	}

	return spec.InstanceID, nil
}

func (e *SvcCatEntangler) constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (smith_v1.Resource, error) {
	serviceInstanceExternalID, err := InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	serviceInstanceSpecBytes, err := e.InstanceSpec(resource, context)
	if err != nil {
		return smith_v1.Resource{}, err
	}
	var parameters *runtime.RawExtension
	if serviceInstanceSpecBytes != nil {
		parameters = &runtime.RawExtension{
			Raw: serviceInstanceSpecBytes,
		}
	}

	var objectMeta meta_v1.ObjectMeta
	if e.ObjectMeta != nil {
		objectMeta, err = e.ObjectMeta(resource, context)
		if err != nil {
			return smith_v1.Resource{}, err
		}
	}

	if objectMeta.Name == "" {
		objectMeta.SetName(wiringutil.ServiceInstanceMetaName(resource.Name))
	}

	var references []smith_v1.Reference
	if e.References != nil {
		references, err = e.References(resource, context)
		if err != nil {
			return smith_v1.Resource{}, err
		}
	}

	return smith_v1.Resource{
		Name:       wiringutil.ServiceInstanceResourceName(resource.Name),
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: objectMeta,
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalID: string(e.ClusterServiceClassExternalID),
						ClusterServicePlanExternalID:  string(e.ClusterServicePlanExternalID),
					},
					Parameters: parameters,
					ExternalID: serviceInstanceExternalID,
				},
			},
		},
	}, nil
}

func (e *SvcCatEntangler) constructResourceContract(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) (*wiringplugin.ResourceContract, error) {
	supportedShapes := []wiringplugin.Shape{
		knownshapes.NewBindableEnvironmentVariables(smithResource.Name),
	}
	optionalShapes, err := e.OptionalShapes(resource, smithResource, context)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute optional shapes for resource %q of type %q", resource.Name, resource.Type)
	}
	supportedShapes = append(supportedShapes, optionalShapes...)
	return &wiringplugin.ResourceContract{
		Shapes: supportedShapes,
	}, nil
}

func (e *SvcCatEntangler) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool, error) {
	if resource.Type != e.ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", resource.Type)
	}

	serviceInstance, err := e.constructServiceInstance(resource, context)
	if err != nil {
		return nil, false, err
	}

	resourceContract, err := e.constructResourceContract(resource, &serviceInstance, context)
	if err != nil {
		return nil, false, err
	}

	result := &wiringplugin.WiringResult{
		Contract:  *resourceContract,
		Resources: []smith_v1.Resource{serviceInstance},
	}

	return result, false, nil
}
