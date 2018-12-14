package microscompute

import (
	"encoding/json"
	"regexp"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretenvvar"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createSecret(spec secretenvvar.Spec, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency) (*core_v1.Secret, error) {
	var ignoreKeyRegex *regexp.Regexp
	if spec.IgnoreKeyRegex != "" {
		regex, err := regexp.Compile(spec.IgnoreKeyRegex)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid ignoreKeyRegex: %q", spec.IgnoreKeyRegex)
		}
		ignoreKeyRegex = regex
	}

	extracted, err := secretenvvar.ExtractEnvironmentVariables(dependencies, ignoreKeyRegex, spec.RenameEnvVar)
	if err != nil {
		return nil, err
	}

	envVarJSONString, err := json.Marshal(map[string]map[string]string{
		spec.OutputJSONKey: extracted,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: core_v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		Data: map[string][]byte{
			spec.OutputSecretKey: envVarJSONString,
		},
	}, nil
}
