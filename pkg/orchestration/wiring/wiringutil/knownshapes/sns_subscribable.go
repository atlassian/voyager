package knownshapes

import (
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
)

const (
	SnsSubscribableShape wiringplugin.ShapeName = "voyager.atl-paas.net/SnsSubscribable"

	// This has to match up with what ends up in the bind credentials for
	// something that emits a topic.
	snsTopicArnOutputNameKey = "TopicArn"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type SnsSubscribable struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   SnsSubscribableData `json:"data"`
}

// +k8s:deepcopy-gen=true
type SnsSubscribableData struct {
	wiringplugin.BindableShapeStruct `json:",inline"`
	TopicArnRef                      wiringutil.BindSecretReference `json:"topicArnRef"`
}

func NewSnsSubscribable(smithResourceName smith_v1.ResourceName, voyagerResourceName voyager.ResourceName) *SnsSubscribable {
	return &SnsSubscribable{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: SnsSubscribableShape,
		},
		Data: SnsSubscribableData{
			BindableShapeStruct: wiringplugin.BindableShapeStruct{
				ServiceInstanceName: wiringplugin.ProtoReference{
					Resource: smithResourceName,
					Path:     "metadata.name",
					Example:  "aname",
				}},
			TopicArnRef: wiringutil.BindSecretReference{
				ProducerResource: voyagerResourceName,
				Path:             fmt.Sprintf("data.%s", snsTopicArnOutputNameKey),
				Example:          `"arn:aws:sns:us-east-1:123456789012:example"`,
			},
		},
	}
}

func (b *SnsSubscribable) Name() wiringplugin.ShapeName {
	return b.ShapeMeta.ShapeName
}
