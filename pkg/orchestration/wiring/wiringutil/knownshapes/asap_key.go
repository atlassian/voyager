package knownshapes

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	ASAPKeyShapeName wiringplugin.ShapeName = "voyager.atl-paas.net/ASAPKey"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type ASAPKey struct {
	wiringplugin.ShapeMeta `json:",inline"`
}

// NewASAPKey creates a new ASAPKey
func NewASAPKey() *ASAPKey {
	return &ASAPKey{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: ASAPKeyShapeName,
		},
	}
}

// Name returns the ASAPKey ShapeName
func (a *ASAPKey) Name() wiringplugin.ShapeName {
	return a.ShapeName
}

// FindASAPKeyShapes returns the first instance of ASAPKey if found
func FindASAPKeyShapes(shapes []wiringplugin.Shape) (*ASAPKey, bool /*found*/, error) {
	typed := &ASAPKey{}
	found, err := FindAndCopyShapeByName(shapes, ASAPKeyShapeName, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
