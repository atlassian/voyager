package saml

import (
	"encoding/json"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  instanceSpec,
			ObjectMeta:                    objectMeta,
			ResourceType:                  ResourceType,
			Shapes:                        shapes,
		},
	}
}

func shapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool, bool, error) {
	bindableEnvVarShape := knownshapes.NewBindableEnvironmentVariablesWithExcludeResourceName(smithResource.Name, ResourcePrefix, map[string]string{
		"SAML_IDP_METADATA_URL": "data.saml_idp_metadata_url",
		"IDP_METADATA_URL":      "data.idp_metadata_url",
	}, true)
	return []wiringplugin.Shape{
		bindableEnvVarShape,
	}, false, false, nil
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool, bool, error) {
	if len(context.Dependencies) > 0 {
		return nil, false, false, errors.New("saml resource should not have any dependencies")
	}
	if resource.Spec == nil {
		// Don't allow user to set anything they shouldn't
		return nil, false, false, errors.New("saml resource is missing user parameters")
	}

	var spec finalSpec

	err := json.Unmarshal(resource.Spec.Raw, &spec)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	// Set default name service--location--resource
	if spec.Name == "" {
		label := ""
		if context.StateContext.Location.Label != "" {
			label = fmt.Sprintf(".%s", context.StateContext.Location.Label)
		}
		spec.Name = fmt.Sprintf("%s--%s.%s.%s%s--%s",
			context.StateContext.ServiceName,
			context.StateContext.Location.Account,
			context.StateContext.Location.EnvType,
			context.StateContext.Location.Region,
			label,
			resource.Name,
		)
	}

	out, err := json.Marshal(&spec)
	return out, false, false, err
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, bool /* externalErr */, bool /* retriableErr */, error) {
	return meta_v1.ObjectMeta{}, false, false, nil
}
