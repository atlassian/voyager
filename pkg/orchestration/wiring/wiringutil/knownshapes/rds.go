package knownshapes

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	RDSShape wiringplugin.ShapeName = "voyager.atl-paas.net/RDS"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type RDS struct {
	wiringplugin.ShapeMeta `json:",inline"`
}

// NewRDS creates a new RDS
func NewRDS() *RDS {
	return &RDS{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: RDSShape,
		},
	}
}

// Name returns the RDS ShapeName
func (a *RDS) Name() wiringplugin.ShapeName {
	return a.ShapeName
}

// FindRDSShapes returns a single instance of the RDS shape if found and will error if there are multiples
func FindRDSShapes(shapes []wiringplugin.Shape) (*RDS, bool /*found*/, error) {
	typed := &RDS{}
	found, err := FindAndCopyShapeByName(shapes, RDSShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
