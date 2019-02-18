package knownshapes

import (
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
)

const (
	SnsSubscribableShape wiringplugin.ShapeName = "voyager.atl-paas.net/SnsSubscribable"

	// This has to match up with what ends up in the bind credentials for
	// something that emits a topic.
	snsTopicArnOutputNameKey       = "TopicArn"
	snsTopicArnReferenceNameSuffix = "topicArn"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SnsSubscribable struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SnsSubscribableData `json:"data"`
}

// +k8s:deepcopy-gen=true
type SnsSubscribableData struct {
	libshapes.BindableShapeStruct `json:",inline"`
	TopicARN                      libshapes.BindingSecretProtoReference `json:"topicArn"`
}

func NewSnsSubscribable(smithResourceName smith_v1.ResourceName) *SnsSubscribable {
	return &SnsSubscribable{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SnsSubscribableShape,
		},
		Data: SnsSubscribableData{
			BindableShapeStruct: libshapes.BindableShapeStruct{
				ServiceInstanceName: libshapes.ProtoReference{
					Resource: smithResourceName,
					Path:     "metadata.name",
					Example:  "aname",
				}},
			TopicARN: libshapes.BindingSecretProtoReference{
				Path:        fmt.Sprintf("data.%s", snsTopicArnOutputNameKey),
				Example:     `"arn:aws:sns:us-east-1:123456789012:example"`,
				NamePostfix: snsTopicArnReferenceNameSuffix,
			},
		},
	}
}

func FindSnsSubscribableShape(shapes []wiringplugin.Shape) (*SnsSubscribable, bool /*found*/, error) {
	typed := &SnsSubscribable{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, SnsSubscribableShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
