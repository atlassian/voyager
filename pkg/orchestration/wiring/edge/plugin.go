package edge

import (
	"encoding/json"
	"github.com/atlassian/voyager"
	orchestration "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	wiring "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
)

const (
	ResourceType voyager.ResourceType = "globaledge"
	ServiceId                         = "10e5a402-45df-5afd-ae86-11377ce2bbb2"
	PlanId                            = "7d57270a-0348-58d3-829d-447a98fe98d5"
	//DeletionDelay                      = 2 * time.Hour
)

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: ServiceId,
			ClusterServicePlanExternalID:  PlanId,
			InstanceSpec:                  getInstanceSpec,
			//ObjectMeta:                    getObjectMeta,
			//References:                    getReferences,
			ResourceType: ResourceType,
		},
	}
}

func getInstanceSpec(resource *orchestration.StateResource, context *wiring.WiringContext) (ret []byte, err error) {
	var params Parameters
	err = json.Unmarshal(resource.Spec.Raw, &params)
	if len(params.UpstreamAddress) == 0 {
		// We need to fill in the upstream from a dependency with DNS
		// We should also include the region from the context.
		//
		// The region acts as a locality tag, so that proxies can match
		// their own location with the nearest upstream
		params.UpstreamAddress = UpstreamAddress{
			{
				Address: "Get FQDN of dependency somehow",
				Region:  context.StateContext.Location.Region,
			},
		}
		// ret = <magical json marshalling with 10 other libraries>
	}
	return
}
