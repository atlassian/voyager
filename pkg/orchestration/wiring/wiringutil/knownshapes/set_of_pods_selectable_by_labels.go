package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
)

const (
	// allows to select set of Kubernetes objects by list of labels
	SetOfPodsSelectableByLabelsShape wiringplugin.ShapeName = "voyager.atl-paas.net/SetOfPodsSelectableByLabelsShape"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SetOfPodsSelectableByLabels struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SetOfPodsSelectableByLabelsData `json:"data"`
}

// +k8s:deepcopy-gen=true
type SetOfPodsSelectableByLabelsData struct {
	DeploymentResourceName smith_v1.ResourceName `json:"deploymentResourceName"`
	Labels                 map[string]string     `json:"labels"`
}

func NewSetOfPodsSelectableByLabels(resourceName smith_v1.ResourceName, labels map[string]string) *SetOfPodsSelectableByLabels {
	return &SetOfPodsSelectableByLabels{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SetOfPodsSelectableByLabelsShape,
		},
		Data: SetOfPodsSelectableByLabelsData{
			DeploymentResourceName: resourceName,
			Labels:                 labels,
		},
	}
}

func FindSetOfPodsSelectableByLabelsShape(shapes []wiringplugin.Shape) (*SetOfPodsSelectableByLabels, bool /*found*/, error) {
	typed := &SetOfPodsSelectableByLabels{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, SetOfPodsSelectableByLabelsShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
