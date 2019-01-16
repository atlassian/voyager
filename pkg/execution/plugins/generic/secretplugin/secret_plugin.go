package secretplugin

import (
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
						"type": "object"
					}
				},
				"additionalProperties": false,
				"required": ["data"]
			}
		`),
	}
}

func (p *Plugin) Process(rawSpec map[string]interface{}, context *smith_plugin.Context) (*smith_plugin.ProcessResult, error) {
	spec := Spec{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSpec, &spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert into typed spec")
	}

	return &smith_plugin.ProcessResult{
		Object: &core_v1.Secret{
			TypeMeta: meta_v1.TypeMeta{
				APIVersion: core_v1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			Data: spec.Data,
		},
	}, nil
}
