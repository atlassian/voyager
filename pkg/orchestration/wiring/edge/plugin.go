package edge

import (
	"encoding/json"
	smith "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orchestration "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	wiring "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "GlobalEdge"
	ServiceId                         = "10e5a402-45df-5afd-ae86-11377ce2bbb2"
	PlanId                            = "7d57270a-0348-58d3-829d-447a98fe98d5"
)

type WiringPlugin struct {
}

func New() *WiringPlugin {
	return &WiringPlugin{}
}

func (p *WiringPlugin) WireUp(resource *orchestration.StateResource, context *wiring.WiringContext) (result *wiring.WiringResult, retriable bool, err error) {
	result = nil
	retriable = false

	if resource.Type != ResourceType {
		err = errors.Errorf("invalid resource type: %q", resource.Type)
		return
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, ServiceId, PlanId)
	if err != nil { return }

	instanceParameters, err := instanceParameters(resource, context)
	if err != nil { return }

	serviceInstance.Spec.Parameters = &runtime.RawExtension{
		Raw: instanceParameters,
	}

	serviceInstanceResource := smith.Resource{
		Name: wiringutil.ServiceInstanceResourceName(resource.Name),
		Spec: smith.ResourceSpec{
			Object: serviceInstance,
		},
	}

	result = &wiring.WiringResult{
		Contract: wiring.ResourceContract{
			Shapes: nil, // Nothing will depend on GlobalEdge
		},
		Resources: []smith.Resource{serviceInstanceResource},
	}
	return
}

func instanceParameters(resource *orchestration.StateResource, context *wiring.WiringContext) ([]byte, error) {
	if resource.Spec == nil {
		return nil, errors.New("empty spec is not allowed")
	}

	// Pass-through SD parameters -> OSB attributes
	var attributes Attributes
	if err := json.Unmarshal(resource.Spec.Raw, &attributes); err != nil {
		return nil, errors.WithStack(err)
	}
	parameters := InstanceParameters{
		ServiceName: string(context.StateContext.ServiceName),
		Resource: ResourceParameters{
			Attributes: attributes,
		},

	}

	if parameters.Resource.Attributes.UpstreamAddress == nil {
		return nil, errors.New("UpstreamAddress is required")

		// TODO: Make upstream address optional and produce it from InternalDNS / KubeIngress output
		// 1. Find dependency of some expected shape ("UpstreamAddressProviderShape" or whatever)
		// 2. Generate a reference to the field from that dependency to be used as an upstream address
		// 3. set UpstreamAddress to this reference inside parameters, i.e.
		// 	parameters.Resource.Attributes.UpstreamAddress = bla
	}

	return json.Marshal(parameters)
}
