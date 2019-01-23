package compute

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/pkg/errors"
)

// ValidateEnvironmentVariables will loop through WiringContext.Dependencies
// ensuring we don't have conflicting environment variables being provided
func ValidateEnvironmentVariables(context *wiringplugin.WiringContext) error {
	envVarCount := make(map[string]int)
	for _, dep := range context.Dependencies {
		shape, found, err := knownshapes.FindEnvironmentVariablesShape(dep.Contract.Shapes)
		if err != nil {
			return errors.Wrap(err, "unable to validate Environment Variable dependencies")
		}

		if found {
			for k := range shape.Data.EnvVars {
				envVarCount[k]++
			}
		}
	}

	for k, count := range envVarCount {
		// Do not allow duplicate environment variable keys
		if count > 1 {
			return errors.New("EnvVar '" + k + "' was provided more than once")
		}
	}
	return nil
}
