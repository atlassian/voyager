package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	LabelledShape wiringplugin.ShapeName = "voyager.atl-paas.net/LabelledShape"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type Labelled struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   LabelledData `json:"data"`
}

// +k8s:deepcopy-gen=true
type LabelledData struct {
	Target smith_v1.ResourceName `json:"target"`
	Labels map[string]string     `json:"labels"`
}

func NewLabelled(resourceName smith_v1.ResourceName, labels map[string]string) *Labelled {
	return &Labelled{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: LabelledShape,
		},
		Data: LabelledData{
			Target: resourceName,
			Labels: labels,
		},
	}
}

func (s *Labelled) Name() wiringplugin.ShapeName {
	return LabelledShape
}
