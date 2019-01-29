package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
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
	SharedDbResourceName     libshapes.ProtoReference `json:"sharedDbResourceName"`
	HasSameRegionReadReplica bool                     `json:"hasSameRegionReadReplica"`
}

func NewSharedDbShape(resourceName smith_v1.ResourceName, hasSameRegionReadReplica bool) *SharedDb {
	return &SharedDb{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SharedDbShape,
		},
		Data: SharedDbData{
			SharedDbResourceName: libshapes.ProtoReference{
				Resource: resourceName,
				Path:     "metadata.name",
				Example:  "myownrds",
			},
			HasSameRegionReadReplica: hasSameRegionReadReplica,
		},
	}
}

func FindSharedDbShape(shapes []wiringplugin.Shape) (*SharedDb, bool /*found*/, error) {
	typed := &SharedDb{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, SharedDbShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
