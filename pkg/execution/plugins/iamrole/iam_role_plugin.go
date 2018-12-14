package iamrole

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

type Plugin struct{}

func New() (smith_plugin.Plugin, error) {
	return &Plugin{}, nil
}

func (p *Plugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name:       "iamrole",
		GVK:        sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"),
		SpecSchema: []byte(schema),
	}
}

// Process processes a plugin specification and produces an object as the result.
func (p *Plugin) Process(rawSpec map[string]interface{}, context *smith_plugin.Context) (*smith_plugin.ProcessResult, error) {
	spec := Spec{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSpec, &spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal json spec")
	}

	// Do the processing
	roleInstance, err := generateRoleInstance(&spec, context.Dependencies)
	if err != nil {
		return nil, err
	}
	return &smith_plugin.ProcessResult{
		Object: roleInstance,
	}, nil
}
