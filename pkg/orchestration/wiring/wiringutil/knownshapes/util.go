package knownshapes

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/pkg/errors"
)

func FindAndCopyShapeByName(shapes []wiringplugin.Shape, name wiringplugin.ShapeName, copyInto wiringplugin.Shape) (bool /*found*/, error) {
	for _, shape := range shapes {
		if shape.Name() == name {
			err := wiringplugin.CopyShape(shape, copyInto)
			if err != nil {
				return false, errors.Wrapf(err, "failed to copy shape %T into %T", shape, copyInto)
			}
			return true, nil
		}
	}
	return false, nil
}
