package secretparameter

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	secretParameterPluginName = "secretparameter"
)

type Plugin struct{}

func New() (smith_plugin.Plugin, error) {
	return &Plugin{}, nil
}

func (p *Plugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name: secretParameterPluginName,
		GVK:  core_v1.SchemeGroupVersion.WithKind("Secret"),
		SpecSchema: []byte(`
			{
				"type": "object",
				"properties": {
					"mapping": {
						"type": "object",
						"minProperties": 1,
						"additionalProperties": {
							"type": "object",
							"additionalProperties": {
								"type": "string",
								"minLength": 1
							}
						}
					}
				},
				"additionalProperties": false,
				"required": ["mapping"]
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

	if len(spec.Mapping) == 0 {
		return &smith_plugin.ProcessResultFailure{
			Error: errors.New("spec is invalid - must provide at least one secret to map"),
		}
	}

	secret, err := createSecret(spec, context.Dependencies)
	if err != nil {
		return &smith_plugin.ProcessResultFailure{
			Error: err,
		}
	}
	return &smith_plugin.ProcessResultSuccess{
		Object: secret,
	}
}
