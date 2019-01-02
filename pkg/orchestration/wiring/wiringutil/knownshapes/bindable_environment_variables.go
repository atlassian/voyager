package knownshapes

import "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"

const (
	BindableEnvironmentVariablesShape wiringplugin.ShapeName = "voyager.atl-paas.net/BindableEnvironmentVariables"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type BindableEnvironmentVariables struct {
	wiringplugin.ShapeMeta           `json:",inline"`
	wiringplugin.BindableShapeStruct `json:",inline"`
	Prefix                           string `json:"prefix,omitempty"`
}

func (s *BindableEnvironmentVariables) Name() wiringplugin.ShapeName {
	return BindableEnvironmentVariablesShape
}
