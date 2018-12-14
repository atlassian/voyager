package secretenvvar

import (
	"regexp"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/execution/plugins"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
)

const (
	// Use Micros1 default ASAP evn vars name to facilitate migration
	asapKeyEnvPrefix     = "ASAPKey"
	asapMicros1EnvPrefix = "ASAP"
	envResourcePrefix    = voyager.Domain + "/envResourcePrefix"
	// CloudFormation can't express _ or -, but we want CF templates to be able to
	// be backwards compatible with other OSB providers, so...
	envDashReplacement = "0DASH0"
)

var (
	envVarRegex = regexp.MustCompile("^[A-Z_]+[A-Z0-9_]*$")
)

// We want to prefix the environment variables with what 'kind' of thing it is.
// Picking this up from an annotation allows wiring plugins to customise this
// (either per instance, or if there are multiple binds per binding).
func findResourcePrefix(binding *sc_v1b1.ServiceBinding, instance *sc_v1b1.ServiceInstance) (string, bool) {
	found := false
	result := ""
	for _, prefix := range []string{
		binding.ObjectMeta.Annotations[envResourcePrefix],
		instance.ObjectMeta.Annotations[envResourcePrefix],
	} {
		if prefix != "" {
			result = prefix
			found = true
			break
		}
	}
	return result, found
}

func makeEnvVarName(elements ...string) string {
	return strings.ToUpper(strings.Replace(strings.Join(elements, "_"), "-", "_", -1))
}

func checkEnvVar(s string) bool {
	return envVarRegex.MatchString(s)
}

func extractPrefixAndSecret(dependency smith_plugin.Dependency) (string, *core_v1.Secret, error) {
	// TODO in the future, if Smith support secret/binding references, it would be nice to complete change
	// this plugin to act like 'envFrom' in a kubernetes container (i.e. take a list of envFromSource
	// with prefixes).
	switch actual := dependency.Actual.(type) {
	case *sc_v1b1.ServiceBinding:
		instance := plugins.FindServiceInstance(actual, dependency.Auxiliary)
		if instance == nil {
			return "", nil, errors.Errorf("missing ServiceInstance %q of ServiceBinding %q in Auxiliary objects",
				actual.Spec.ServiceInstanceRef.Name, actual.Name)
		}
		secret := plugins.FindBindingSecret(actual, dependency.Outputs)
		if secret == nil {
			return "", nil, errors.Errorf("missing Secret for ServiceBinding %q", actual.Name)
		}
		resourcePrefix, ok := findResourcePrefix(actual, instance)
		if !ok {
			return "", nil, errors.Errorf("prefix annotation for %q present and empty for ServiceInstance %q",
				envResourcePrefix, instance.Name)
		}
		if resourcePrefix == "" {
			return instance.Name, secret, nil
		}
		if resourcePrefix == asapKeyEnvPrefix {
			return asapMicros1EnvPrefix, secret, nil
		}
		return resourcePrefix + "_" + instance.Name, secret, nil
		// Ultimately, this is probably not that useful (in that it will only process secrets set
		// in the bundle, which means they're not that secret...). Unfortunately, the plugin does
		// not have access to arbitrary secrets in the namespace.
	case *core_v1.Secret:
		return actual.Name, actual, nil
	default:
		return "", nil, errors.Errorf("unsupported dependency object kind - got: %q, expected ServiceBinding or Secret",
			actual.GetObjectKind().GroupVersionKind())
	}
}

func renameEnvironmentVariables(renameMap map[string]string, environmentVariables map[string]string) (map[string]string, error) {

	if len(renameMap) == 0 {
		return environmentVariables, nil
	}

	newEnvVars := make(map[string]string, len(environmentVariables))
	copiedRenameMap := make(map[string]string, len(renameMap))

	for k, v := range renameMap {
		copiedRenameMap[k] = v
	}

	for originalKey, v := range environmentVariables {
		// Rename all remaining keys
		keyToUse := originalKey
		if renamedKey, shouldRename := copiedRenameMap[originalKey]; shouldRename {
			keyToUse = renamedKey
			delete(copiedRenameMap, originalKey)
		}

		if _, ok := newEnvVars[keyToUse]; ok {
			return nil, errors.Errorf("key %q already exists in environment variables, cannot rename", keyToUse)
		}

		newEnvVars[keyToUse] = v
	}

	// check environment variable existance for renames
	if len(copiedRenameMap) != 0 {
		keys := make([]string, 0, len(copiedRenameMap))
		for k := range copiedRenameMap {
			keys = append(keys, k)
		}
		return nil, errors.Errorf("environment varibles do not exist and cannot be renamed: %v", strings.Join(keys, ", "))
	}

	return newEnvVars, nil
}

func ExtractEnvironmentVariables(dependencies map[smith_v1.ResourceName]smith_plugin.Dependency, ignoreKeyRegex *regexp.Regexp, renameMap map[string]string) (map[string]string, error) {
	merged := make(map[string]string)

	for _, dependency := range dependencies {
		prefix, secret, err := extractPrefixAndSecret(dependency)
		if err != nil {
			return nil, err
		}

		for key, value := range secret.Data {
			// See comment on constant
			compatKey := strings.Replace(key, envDashReplacement, "-", -1)
			envVarName := makeEnvVarName(prefix, compatKey)

			if !checkEnvVar(envVarName) {
				return nil, errors.Errorf("invalid environment variable name %s", envVarName)
			}

			if ignoreKeyRegex != nil && ignoreKeyRegex.MatchString(envVarName) {
				continue
			}

			if _, ok := merged[envVarName]; ok {
				return nil, errors.Errorf("duplicate environment variable %s", envVarName)
			}

			merged[envVarName] = string(value)
		}

	}

	return renameEnvironmentVariables(renameMap, merged)
}
