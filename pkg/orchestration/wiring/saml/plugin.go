package saml

import (
	"encoding/json"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	clusterServiceClassExternalID                      = "277d87d1-4cf6-4d96-8bd6-3affb551f21c"
	clusterServicePlanExternalID                       = "55beb5f4-16f0-4fe5-abcb-95e077271bde"
	ResourceType                  voyager.ResourceType = "SAML"
	ResourcePrefix                                     = "SAML"
)

type autowiringOnlySpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
}
type finalSpec struct {
	autowiringOnlySpec
}
type WiringPlugin struct {
}

func New() *WiringPlugin {
	return &WiringPlugin{}
}

func (p *WiringPlugin) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, clusterServiceClassExternalID, clusterServicePlanExternalID)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	instanceParameters, external, retriable, err := instanceParameters(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}
	serviceInstance.Spec.Parameters = &runtime.RawExtension{
		Raw: instanceParameters,
	}

	instanceResourceName := wiringutil.ServiceInstanceResourceName(resource.Name)

	smithResource := smith_v1.Resource{
		Name:       instanceResourceName,
		References: nil,
		Spec: smith_v1.ResourceSpec{
			Object: serviceInstance,
		},
	}

	shapes, external, retriable, err := instanceShapes(&smithResource)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: shapes,
		},
		Resources: []smith_v1.Resource{smithResource},
	}
}

func instanceShapes(smithResource *smith_v1.Resource) ([]wiringplugin.Shape, bool, bool, error) {
	bindableEnvVarShape := knownshapes.NewBindableEnvironmentVariablesWithExcludeResourceName(smithResource.Name, ResourcePrefix, map[string]string{
		"SAML_IDP_METADATA_URL": "data.saml_idp_metadata_url",
		"IDP_METADATA_URL":      "data.idp_metadata_url",
	}, true)
	return []wiringplugin.Shape{
		bindableEnvVarShape,
	}, false, false, nil
}

func instanceParameters(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* externalErr */, bool /* retriable */, error) {
	if len(context.Dependencies) > 0 {
		return nil, true, false, errors.New("saml resource should not have any dependencies")
	}
	if resource.Spec == nil {
		return nil, true, false, errors.New("saml resource is missing user parameters")
	}

	var spec finalSpec

	err := json.Unmarshal(resource.Spec.Raw, &spec)
	if err != nil {
		return nil, true, false, errors.WithStack(err)
	}

	// Set default name service--location--resource
	if spec.Name == "" {
		label := ""
		if context.StateContext.Location.Label != "" {
			label = fmt.Sprintf(".%s", context.StateContext.Location.Label)
		}
		spec.Name = fmt.Sprintf("%s--%s.%s.%s%s--%s",
			context.StateContext.ServiceName,
			context.StateContext.Location.EnvType,
			context.StateContext.Location.Account,
			context.StateContext.Location.Region,
			label,
			resource.Name,
		)
	}

	out, err := json.Marshal(&spec)
	return out, false, false, err
}
