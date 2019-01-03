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
type bindableIamAccessible struct {
	wiringplugin.ShapeMeta           `json:",inline"`
	wiringplugin.BindableShapeStruct `json:",inline"`
	//IAMRoleARN    BindingProtoReference
	//IAMProfileARN BindingProtoReference
	IAMPolicySnippet wiringplugin.BindingProtoReference
}

func NewBindableIamAccessible(resourceName smith_v1.ResourceName, IAMPolicySnippetPath string) *bindableIamAccessible {
	return &bindableIamAccessible{
		ShapeMeta: wiringplugin.ShapeMeta{BindableIamAccessibleShape},
		BindableShapeStruct: wiringplugin.BindableShapeStruct{
			ServiceInstanceName: wiringplugin.ProtoReference{
				Resource: resourceName,
				Path:     "metadata.name",
			}},
		IAMPolicySnippet: wiringplugin.BindingProtoReference{Path: IAMPolicySnippetPath},
	}
}

func (s *bindableIamAccessible) Name() wiringplugin.ShapeName {
	return BindableIamAccessibleShape
}
