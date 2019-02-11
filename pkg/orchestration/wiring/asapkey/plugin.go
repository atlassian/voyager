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
	ResourcePrefix                                     = "ASAP"
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
			Shapes:                        shapes,
		},
	}
}

func shapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* externalErr */, bool /* retriableErr */, error) {
	bindableEnvVarShape := knownshapes.NewBindableEnvironmentVariablesWithExcludeResourceName(smithResource.Name, ResourcePrefix, map[string]string{
		"PRIVATE_KEY": "data.private_key",
		"ISSUER":      "data.issuer",
		"KEY_ID":      "data.key_id",
		"AUDIENCE":    "data.audience",
	}, true)
	return []wiringplugin.Shape{
		bindableEnvVarShape,
		knownshapes.NewASAPKey(),
	}, false, false, nil
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* externalErr */, bool /* retriableErr */, error) {
	if len(context.Dependencies) > 0 {
		// this is an external error - the dependencies are wrong
		return nil, true, false, errors.New("asap key should not have any dependencies")
	}
	if resource.Spec != nil {
		// this is an external error, the user provided a spec which they should not have
		return nil, true, false, errors.Errorf("asap key does not accept any user parameters")
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
	result, err := json.Marshal(&spec)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}
	return result, false, false, nil
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, bool /* externalErr */, bool /* retriableErr */, error) {
	return meta_v1.ObjectMeta{}, false, false, nil
}
