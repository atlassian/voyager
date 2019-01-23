package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	// TODO: DOCUMENT THE DESIRED SEMANTICS. Overlap
	EnvironmentVariablesShape wiringplugin.ShapeName = "voyager.atl-paas.net/EnvironmentVariables"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type EnvironmentVariables struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   EnvironmentVariablesData `json:"data"`
}

// +k8s:deepcopy-gen=true
type EnvironmentVariablesData struct {
	EnvVars map[string][]byte `json:"envVars,omitempty"`
	// TODO: In future we want to support secrets as well
	// SecretEnvVars map[string][]byte `json:secretEnvVars,omitempty"`
}

func NewEnvironmentVariables(resourceName smith_v1.ResourceName, envVars map[string][]byte /*, secretEnvVars map[string][]byte*/) *EnvironmentVariables {
	return &EnvironmentVariables{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: EnvironmentVariablesShape,
		},
		Data: EnvironmentVariablesData{
			EnvVars: envVars,
			// SecretEnvVars: secretEnvVars,
		},
	}
}

func (b *EnvironmentVariables) Name() wiringplugin.ShapeName {
	return b.ShapeName
}

func FindEnvironmentVariablesShape(shapes []wiringplugin.Shape) (*EnvironmentVariables, bool /*found*/, error) {
	typed := &EnvironmentVariables{}
	found, err := FindAndCopyShapeByName(shapes, EnvironmentVariablesShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
