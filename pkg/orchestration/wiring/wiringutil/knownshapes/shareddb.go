package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	SharedDbShape wiringplugin.ShapeName = "voyager.atl-paas.net/SharedDb"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SharedDb struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SharedDbData `json:"data"`
}

// +k8s:deepcopy-gen=true
type SharedDbData struct {
	SharedDbResourceName     wiringplugin.ProtoReference `json:"sharedDbResourceName"`
	HasSameRegionReadReplica bool                        `json:"hasSameRegionReadReplica"`
}

func NewSharedDbShape(resourceName smith_v1.ResourceName, hasSameRegionReadReplica bool) *SharedDb {
	return &SharedDb{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SharedDbShape,
		},
		Data: SharedDbData{
			SharedDbResourceName: wiringplugin.ProtoReference{
				Resource: resourceName,
				Path:     "metadata.name",
				Example:  "myownrds",
			},
			HasSameRegionReadReplica: hasSameRegionReadReplica,
		},
	}
}

func (b SharedDb) Name() wiringplugin.ShapeName {
	return b.ShapeMeta.ShapeName
}

func FindSharedDbShape(shapes []wiringplugin.Shape) (*SharedDb, bool /*found*/, error) {
	typed := &SharedDb{}
	found, err := FindAndCopyShapeByName(shapes, SharedDbShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
