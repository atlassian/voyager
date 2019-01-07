package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

const (
	// BindableIamAccessibleShape is a When you bind to it it returns IAM policy snippet.
	BindableIamAccessibleShape wiringplugin.ShapeName = "voyager.atl-paas.net/BindableIamAccessible"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type BindableIamAccessible struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   BindableIamAccessibleData `json:"data"`
}

// +k8s:deepcopy-gen=true
type BindableIamAccessibleData struct {
	wiringplugin.BindableShapeStruct `json:",inline"`
	//IAMRoleARN    BindingProtoReference
	//IAMProfileARN BindingProtoReference
	IAMPolicySnippet wiringplugin.BindingProtoReference
}

func NewBindableIamAccessible(resourceName smith_v1.ResourceName, IAMPolicySnippetPath string) *BindableIamAccessible {
	return &BindableIamAccessible{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: BindableIamAccessibleShape,
		},
		Data: BindableIamAccessibleData{
			BindableShapeStruct: wiringplugin.BindableShapeStruct{
				ServiceInstanceName: wiringplugin.ProtoReference{
					Resource: resourceName,
					Path:     "metadata.name",
					Example:  "aname",
				}},
			IAMPolicySnippet: wiringplugin.BindingProtoReference{Path: IAMPolicySnippetPath},
		},
	}
}

func (s *BindableIamAccessible) Name() wiringplugin.ShapeName {
	return BindableIamAccessibleShape
}
