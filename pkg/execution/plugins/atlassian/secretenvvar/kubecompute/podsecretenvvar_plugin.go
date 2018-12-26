package kubecompute

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	podSecretEnvVarPluginName = "podsecretenvvar"
)

type PodSecretEnvVarPlugin struct{}

func New() (smith_plugin.Plugin, error) {
	return &PodSecretEnvVarPlugin{}, nil
}

func (p *PodSecretEnvVarPlugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name: podSecretEnvVarPluginName,
		GVK:  core_v1.SchemeGroupVersion.WithKind("Secret"),
		SpecSchema: []byte(`
			{
				"type": "object",
				"properties": {
					"ignoreKeyRegex": {
						"type": "string",
						"minLength": 1
					},
					"rename": {
						"type": "object",
						"additionalProperties": {
							"type": "string"
						}
					}
				},
				"additionalProperties": false
			}
		`),
	}
}

func (p *PodSecretEnvVarPlugin) Process(rawSpec map[string]interface{}, context *smith_plugin.Context) (*smith_plugin.ProcessResult, error) {
	spec := secretenvvar.PodSpec{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSpec, &spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert into typed spec")
	}

	secret, err := createSecret(&spec, context.Dependencies)
	if err != nil {
		return nil, err
	}
	return &smith_plugin.ProcessResult{
		Object: secret,
	}, nil
}
