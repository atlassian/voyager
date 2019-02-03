package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	// allows to select set of Kubernetes objects by list of scaling
	SetOfScalingShape wiringplugin.ShapeName = "voyager.atl-paas.net/SetOfScalingShape"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SetOfScaling struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SetOfScalingData `json:"data"`
}

// +k8s:deepcopy-gen=true
type Scaling struct {
	MinReplicas int32 `json:"minReplicas,omitempty"`
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	CPULimit    int32 `json:"cpuLimit,omitempty"`
	MemoryLimit int32 `json:"memoryLimit,omitempty"`
}

// +k8s:deepcopy-gen=true
type SetOfScalingData struct {
	DeploymentResourceName smith_v1.ResourceName `json:"deploymentResourceName"`
	Scaling                Scaling               `json:"scaling"`
}

func NewSetOfScaling(resourceName smith_v1.ResourceName, scaling Scaling) *SetOfScaling {
	return &SetOfScaling{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SetOfScalingShape,
		},
		Data: SetOfScalingData{
			DeploymentResourceName: resourceName,
			Scaling:                scaling,
		},
	}
}

func (s *SetOfScaling) Name() wiringplugin.ShapeName {
	return s.ShapeName
}

func FindSetOfPScalingShape(shapes []wiringplugin.Shape) (*SetOfScaling, bool /*found*/, error) {
	typed := &SetOfScaling{}
	found, err := FindAndCopyShapeByName(shapes, SetOfScalingShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
