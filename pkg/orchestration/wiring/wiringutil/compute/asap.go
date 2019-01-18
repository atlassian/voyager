package compute

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/pkg/errors"
)

// ValidateASAPDependencies will loop through WiringContext.Dependencies ensuring only one ASAPKey shape is found
func ValidateASAPDependencies(context *wiringplugin.WiringContext) error {
	asapDependencyCount := 0
	for _, dep := range context.Dependencies {
		_, found, err := knownshapes.FindASAPKeyShapes(dep.Contract.Shapes)
		if err != nil {
			return errors.Wrap(err, "unable to validate ASAP dependencies")
		}

		if found {
			// Only allow one asap key dependency per compute
			// so we can use same micros1 env var names and facilitate migration
			if asapDependencyCount++; asapDependencyCount > 1 {
				return errors.New("cannot depend on more than one asap key resource")
			}
		}
	}
	return nil
}
