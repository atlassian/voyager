package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
)

const (
	// allows to select set of Kubernetes objects by list of datadog
	SetOfDatadogShape wiringplugin.ShapeName = "voyager.atl-paas.net/SetOfDatadogShape"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SetOfDatadog struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SetOfDatadogData `json:"data"`
}

// +k8s:deepcopy-gen=true
type SetOfDatadogData struct {
	DeploymentResourceName smith_v1.ResourceName `json:"deploymentResourceName"`
}

func NewSetOfDatadog(resourceName smith_v1.ResourceName) *SetOfDatadog {
	return &SetOfDatadog{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SetOfDatadogShape,
		},
		Data: SetOfDatadogData{
			DeploymentResourceName: resourceName,
		},
	}
}

func FindSetOfDatadogShape(shapes []wiringplugin.Shape) (*SetOfDatadog, bool /*found*/, error) {
	typed := &SetOfDatadog{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, SetOfDatadogShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
