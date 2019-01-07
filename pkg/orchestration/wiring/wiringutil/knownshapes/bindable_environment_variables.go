package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	BindableEnvironmentVariablesShape wiringplugin.ShapeName = "voyager.atl-paas.net/BindableEnvironmentVariables"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type BindableEnvironmentVariables struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   BindableEnvironmentVariablesData `json:"data"`
}

// +k8s:deepcopy-gen=true
type BindableEnvironmentVariablesData struct {
	wiringplugin.BindableShapeStruct `json:",inline"`
	Prefix                           string `json:"prefix,omitempty"`
}

func NewBindableEnvironmentVariables(resourceName smith_v1.ResourceName) *BindableEnvironmentVariables {
	return &BindableEnvironmentVariables{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: BindableEnvironmentVariablesShape,
		},
		Data: BindableEnvironmentVariablesData{
			BindableShapeStruct: wiringplugin.BindableShapeStruct{
				ServiceInstanceName: wiringplugin.ProtoReference{
					Resource: resourceName,
					Path:     "metadata.name",
					Example:  "aname",
				}},
		},
	}
}

func (b *BindableEnvironmentVariables) Name() wiringplugin.ShapeName {
	return b.ShapeMeta.ShapeName
}
