package kubecompute

import (
	"regexp"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createSecret(spec *secretenvvar.PodSpec, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency) (*core_v1.Secret, error) {
	var ignoreKeyRegex *regexp.Regexp
	if spec.IgnoreKeyRegex != "" {
		regex, err := regexp.Compile(spec.IgnoreKeyRegex)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid ignoreKeyRegex: %q", spec.IgnoreKeyRegex)
		}
		ignoreKeyRegex = regex
	}

	environmentVariables, err := secretenvvar.ExtractEnvironmentVariables(dependencies, ignoreKeyRegex, spec.RenameEnvVar)
	if err != nil {
		return nil, err
	}

	secretData := make(map[string][]byte, len(environmentVariables))
	for key, val := range environmentVariables {
		secretData[key] = []byte(val)
	}

	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		Data: secretData,
	}, nil
}
