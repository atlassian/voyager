package compute

import (
	"crypto"
	"encoding/hex"
	"io"
	"sort"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/pkg/errors"
)

type ResourceWithEnvVarBinding struct {
	ResourceName        voyager.ResourceName
	ResourceType        voyager.ResourceType
	BindableEnvVarShape knownshapes.BindableEnvironmentVariables
	BindingName         smith_v1.ResourceName
}

var (
	envVarReplacer = strings.NewReplacer("-", "_", ".", "_")
)

func GenerateEnvVars(renameEnvVar map[string]string, bindingResults []ResourceWithEnvVarBinding) ([]smith_v1.Reference, map[string]string, error) {
	originalEnvVars := map[string]string{}
	var dependencyReferences []smith_v1.Reference

	for _, bindingResult := range bindingResults {
		prefix := bindingResult.BindableEnvVarShape.Data.Prefix
		bindingName := bindingResult.BindingName
		resourceName := bindingResult.ResourceName

		for envVarKey, path := range bindingResult.BindableEnvVarShape.Data.Vars {
			smithReference := smith_v1.Reference{
				Name:     wiringutil.ReferenceName(bindingName, makeRefPathSuffix(path)),
				Resource: bindingName,
				Path:     path,
				Modifier: smith_v1.ReferenceModifierBindSecret,
			}

			// Create the environment name {PREFIX}_{RESOURCE_NAME}_{KEY}
			// Fail on any clashes
			envVarNameSlice := []string{prefix}
			if !bindingResult.BindableEnvVarShape.Data.ExcludeResourceNameInKey {
				envVarNameSlice = append(envVarNameSlice, string(resourceName))
			}
			envVarNameSlice = append(envVarNameSlice, envVarKey)
			envVarName := makeEnvVarName(envVarNameSlice...)
			_, exists := originalEnvVars[envVarName]
			if exists {
				return nil, nil, errors.Errorf("clashing environment variable %q", envVarName)
			}

			originalEnvVars[envVarName] = smithReference.Ref()

			dependencyReferences = append(dependencyReferences, smithReference)
		}
	}

	envVars, err := renameEnvironmentVariables(renameEnvVar, originalEnvVars)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// sort the dependecy references to make the output stable
	sort.Slice(dependencyReferences, func(i, j int) bool {
		return dependencyReferences[i].Name < dependencyReferences[j].Name
	})

	return dependencyReferences, envVars, nil

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

	// check environment variable existence for renames
	if len(copiedRenameMap) != 0 {
		keys := make([]string, 0, len(copiedRenameMap))
		for k := range copiedRenameMap {
			keys = append(keys, k)
		}
		return nil, errors.Errorf("environment variables do not exist and cannot be renamed: %s", strings.Join(keys, ", "))
	}

	return newEnvVars, nil
}

func makeEnvVarName(elements ...string) string {
	return strings.ToUpper(envVarReplacer.Replace(strings.Join(elements, "_")))
}

func makeRefPathSuffix(path string) string {
	hash := crypto.SHA1.New()
	io.WriteString(hash, path) // nolint: gosec, errcheck
	return hex.EncodeToString(hash.Sum(nil))
}
