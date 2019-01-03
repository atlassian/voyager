package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	BindableEnvironmentVariablesShape wiringplugin.ShapeName = "voyager.atl-paas.net/BindableEnvironmentVariables"
)

type ServiceInstanceReference interface {
	Reference() wiringplugin.ProtoReference
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type bindableEnvironmentVariables struct {
	wiringplugin.ShapeMeta           `json:",inline"`
	wiringplugin.BindableShapeStruct `json:",inline"`
	Prefix                           string `json:"prefix,omitempty"`
}

func NewBindableEnvironmentVariables(resourceName smith_v1.ResourceName) *bindableEnvironmentVariables {
	return &bindableEnvironmentVariables{
		ShapeMeta: wiringplugin.ShapeMeta{BindableEnvironmentVariablesShape},
		BindableShapeStruct: wiringplugin.BindableShapeStruct{
			ServiceInstanceName: wiringplugin.ProtoReference{
				Resource: resourceName,
				Path:     "metadata.name",
			}},
	}
}

func (b *bindableEnvironmentVariables) Name() wiringplugin.ShapeName {
	return b.ShapeMeta.ShapeName
}

func (b *bindableEnvironmentVariables) Reference() wiringplugin.ProtoReference {
	return b.BindableShapeStruct.ServiceInstanceName
}
