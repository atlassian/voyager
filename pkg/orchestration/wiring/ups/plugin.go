package ups

import (
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
)

const (
	ResourceType voyager.ResourceType = "UPS"

	clusterServiceClassExternalID = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468"
	clusterServicePlanExternalID  = "86064792-7ea2-467b-af93-ac9694d96d52"
)

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  InstanceSpec,
			ResourceType:                  ResourceType,
			OptionalShapes:                svccatentangler.NoOptionalShapes,
		},
	}
}

// Just a straight passthrough...
// (should probably just implement a default autowiring function similar to how RPS OSB works, which
// takes the class/plan names as arguments?)
func InstanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	if resource.Spec == nil {
		return nil, nil
	}

	return resource.Spec.Raw, nil
}
