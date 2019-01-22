package secretplugin

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	PluginName smith_v1.PluginName = "secret"
)

type Plugin struct{}

func New() (smith_plugin.Plugin, error) {
	return &Plugin{}, nil
}

func (p *Plugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name: PluginName,
		GVK:  core_v1.SchemeGroupVersion.WithKind("Secret"),
		SpecSchema: []byte(`
			{
				"type": "object",
				"properties": {
					"data": {
						"type": "object",
						"additionalProperties": { "type": "string" }
					},
					"jsondata": {
						"type": "object"
					}
				},
				"additionalProperties": false,
				"oneOf": [
					{ "required" : [ "data" ] },
					{ "required" : [ "jsondata" ] }
				]
			}
		`),
	}
}

func (p *Plugin) Process(rawSpec map[string]interface{}, context *smith_plugin.Context) smith_plugin.ProcessResult {
	spec := Spec{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSpec, &spec)
	if err != nil {
		return &smith_plugin.ProcessResultFailure{
			Error: errors.Wrap(err, "failed to convert into typed spec"),
		}
	}

	// Append to final output
	converted := map[string][]byte{}
	for k, v := range spec.JSONData {
		envVarJSONString, err := json.Marshal(v)
		if err != nil {
			return &smith_plugin.ProcessResultFailure{
				Error: errors.WithStack(err),
			}
		}
		converted[k] = envVarJSONString
	}
	for k, v := range spec.Data {
		converted[k] = []byte(v)
	}

	return &smith_plugin.ProcessResultSuccess{
		Object: &core_v1.Secret{
			TypeMeta: meta_v1.TypeMeta{
				APIVersion: core_v1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			Data: converted,
		},
	}
}
