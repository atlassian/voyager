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

type SvcCatEntangler struct {
	ClusterServiceClassExternalID servicecatalog.ClassExternalID
	ClusterServicePlanExternalID  servicecatalog.PlanExternalID

	InstanceSpec func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]byte, error)
	ObjectMeta   func(*orch_v1.StateResource, *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error)
	References   func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]smith_v1.Reference, error)

	ResourceType           voyager.ResourceType
	OutputResourceContract bool
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

func (e *SvcCatEntangler) constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (wiringplugin.WiredSmithResource, error) {
	serviceInstanceExternalID, err := InstanceID(resource.Spec)
	if err != nil {
		return wiringplugin.WiredSmithResource{}, err
	}
	serviceInstanceSpecBytes, err := e.InstanceSpec(resource, context)
	if err != nil {
		return wiringplugin.WiredSmithResource{}, err
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
			return wiringplugin.WiredSmithResource{}, err
		}
	}

	if objectMeta.Name == "" {
		objectMeta.SetName(wiringutil.ServiceInstanceMetaName(resource.Name))
	}

	var references []smith_v1.Reference
	if e.References != nil {
		references, err = e.References(resource, context)
		if err != nil {
			return wiringplugin.WiredSmithResource{}, err
		}
	}

	return wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
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
		},
		Exposed: !e.OutputResourceContract,
	}, nil
}

func (e *SvcCatEntangler) constructResourceContract(resource *orch_v1.StateResource, smithResource smith_v1.Resource, context *wiringplugin.WiringContext) (wiringplugin.ResourceContract, error) {
	// TODO(kopper): Actually implement.
	return wiringplugin.ResourceContract{
		Shapes: []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResource.Name),
			knownshapes.NewBindableIamAccessible(smithResource.Name, "IamPolicySnippet"),
		},
		//Data: []wiringplugin.DataItem{
		//	{
		//		Name: "fakeNotEmptyContract",
		//	},
		//},
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

	var result *wiringplugin.WiringResult
	if e.OutputResourceContract {
		resourceContract, err := e.constructResourceContract(resource, serviceInstance.SmithResource, context)
		if err != nil {
			return nil, false, err
		}
		result = &wiringplugin.WiringResult{
			Contract:  resourceContract,
			Resources: []wiringplugin.WiredSmithResource{serviceInstance},
		}
	} else {
		result = &wiringplugin.WiringResult{
			Resources: []wiringplugin.WiredSmithResource{serviceInstance},
		}
	}

	return result, false, nil
}
