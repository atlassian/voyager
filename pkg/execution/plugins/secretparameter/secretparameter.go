package secretparameter

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createSecret(spec Spec, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency) (*core_v1.Secret, error) {
	outputSecretMap := make(map[string][]byte, len(spec.Mapping))

	for smithResourceName, mappingRules := range spec.Mapping {
		dependency, ok := dependencies[smithResourceName]
		if !ok {
			return nil, errors.Errorf("unknown resource %q in mappings", smithResourceName)
		}
		secret, err := extractSecret(dependency)
		if err != nil {
			return nil, err
		}

		parameters, err := convertSecretToParameters(mappingRules, secret)
		if err != nil {
			return nil, err
		}
		outputSecretMap[string(smithResourceName)], err = json.Marshal(parameters)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		Data: outputSecretMap,
	}, nil
}

func extractSecret(dependency smith_plugin.Dependency) (*core_v1.Secret, error) {
	switch actual := dependency.Actual.(type) {
	case *sc_v1b1.ServiceBinding:
		secret := plugins.FindBindingSecret(actual, dependency.Outputs)
		if secret == nil {
			return nil, errors.Errorf("missing secret for ServiceBinding %q", actual.Name)
		}
		return secret, nil
	case *core_v1.Secret:
		return actual, nil
	default:
		return nil, errors.Errorf("unsupported dependency object kind - got: %s, expected ServiceBinding or Secret",
			actual.GetObjectKind().GroupVersionKind())
	}
}

func convertSecretToParameters(mappingRules map[string]string, secret *core_v1.Secret) (map[string]string, error) {
	parameters := make(map[string]string, len(mappingRules))

	for secretKey, outputKey := range mappingRules {
		inputValue, ok := secret.Data[secretKey]
		if !ok {
			return nil, errors.Errorf("missing requested secret key %q", secretKey)
		}
		parameters[outputKey] = string(inputValue)
	}

	return parameters, nil
}
