package microscompute

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretenvvar"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	secretEnvVarPluginName = "secretenvvar"
)

type SecretEnvVarPlugin struct{}

func New() (smith_plugin.Plugin, error) {
	return &SecretEnvVarPlugin{}, nil
}

func (p *SecretEnvVarPlugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name: secretEnvVarPluginName,
		GVK:  core_v1.SchemeGroupVersion.WithKind("Secret"),
		SpecSchema: []byte(`
			{
				"type": "object",
				"properties": {
					"outputSecretKey": {
						"type": "string",
						"minLength": 1
					},
					"outputJsonKey": {
						"type": "string",
						"minLength": 1
					},
					"rename": {
						"type": "object",
						"additionalProperties": {
							"type": "string"
						}
					},
					"ignoreKeyRegex": {
						"type": "string",
						"minLength": 1
					}
				},
				"additionalProperties": false,
				"required": ["outputSecretKey", "outputJsonKey"]
			}
		`),
	}
}

func (p *SecretEnvVarPlugin) Process(rawSpec map[string]interface{}, context *smith_plugin.Context) (*smith_plugin.ProcessResult, error) {
	spec := secretenvvar.Spec{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSpec, &spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert into typed spec")
	}

	if spec.OutputSecretKey == "" || spec.OutputJSONKey == "" {
		return nil, errors.New("spec is invalid - must have both outputSecretKey and outputJsonKey")
	}

	secret, err := createSecret(spec, context.Dependencies)
	if err != nil {
		return nil, err
	}
	return &smith_plugin.ProcessResult{
		Object: secret,
	}, nil
}
