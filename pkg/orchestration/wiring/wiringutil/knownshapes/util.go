package knownshapes

import (
	"fmt"

	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/pkg/errors"
)

// FindAndCopyShapeByName iterates over a given array of Shapes, finding one based on a given name and will error if the given name belongs to multiple shapes
func FindAndCopyShapeByName(shapes []wiringplugin.Shape, name wiringplugin.ShapeName, copyInto wiringplugin.Shape) (bool /*found*/, error) {
	found := false
	for _, shape := range shapes {
		if shape.Name() == name {
			// Ensure we only have one of the same shape
			if found {
				return found, fmt.Errorf("found multiple shapes with name %s", name)
			}
			found = true
			err := wiringplugin.CopyShape(shape, copyInto)
			if err != nil {
				return found, errors.Wrapf(err, "failed to copy shape %T into %T", shape, copyInto)
			}
		}
	}
	return found, nil
}
