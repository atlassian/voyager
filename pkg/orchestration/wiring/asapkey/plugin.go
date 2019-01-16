package asapkey

import (
	"encoding/json"

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
	clusterServiceClassExternalID                      = "daa6e8e7-7201-4031-86f2-ef9fdfeae7d6"
	clusterServicePlanExternalID                       = "07bb749b-3500-454a-87cd-1244534083f0"
	RepositoryEnvVarName                               = "ASAP_PUBLIC_KEY_REPOSITORY_URL"
	RepositoryFallbackEnvVarName                       = "ASAP_PUBLIC_KEY_FALLBACK_REPOSITORY_URL"
	RepositoryProd                                     = "https://asap-distribution.us-west-1.prod.paas-inf.net/"
	RepositoryFallbackProd                             = "https://asap-distribution.us-west-1.prod.paas-inf.net/"
	RepositoryStg                                      = "https://asap-distribution.us-west-1.staging.paas-inf.net/"
	RepositoryFallbackStg                              = "https://asap-distribution.us-east-1.staging.paas-inf.net/"
	ResourceType                  voyager.ResourceType = "ASAPKey"
)

type autowiringOnlySpec struct {
	KeyName     string `json:"keyName"`
	Creator     string `json:"creator"`
	ServiceName string `json:"serviceName"`
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
			OptionalShapes:                optionalShapes,
		},
	}
}

// optionalShapes returns a list of Shapes that the ASAPKey wiring plugin could output
func optionalShapes(_ *orch_v1.StateResource, smithResource *smith_v1.Resource, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, error) {
	return []wiringplugin.Shape{
		knownshapes.NewASAPKey(smithResource.Name),
	}, nil
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	if len(context.Dependencies) > 0 {
		return nil, errors.New("asap key should not have any dependencies")
	}
	if resource.Spec != nil {
		// Don't allow user to set anything they shouldn't
		return nil, errors.Errorf("asap key does not accept any user parameters")
	}
	var spec finalSpec
	// the issuer name is calculated by first combining the Micros2 serviceName with the ASAPKey resource name
	// the keyserver will prefix this value it with "micros/" when binding the resource
	// example: Micros2 serviceName=foo, ASAPKey resource=my-asap, final issuer "micros/foo/my-asap"
	spec.ServiceName = string(context.StateContext.ServiceName) + "/" + string(resource.Name)
	spec.KeyName = string(resource.Name)
	// creator is just a stored description of which entity created the ASAP key pair on keyserver side
	// not used anywhere
	spec.Creator = "micros2"
	return json.Marshal(&spec)
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error) {
	return meta_v1.ObjectMeta{
		Annotations: map[string]string{
			voyager.Domain + "/envResourcePrefix": string(ResourceType),
		},
	}, nil
}
