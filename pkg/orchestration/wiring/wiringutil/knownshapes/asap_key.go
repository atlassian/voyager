package knownshapes

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
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

// FindASAPKeyShapes returns the first instance of ASAPKey if found
func FindASAPKeyShapes(shapes []wiringplugin.Shape) (*ASAPKey, bool /*found*/, error) {
	typed := &ASAPKey{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, ASAPKeyShapeName, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
