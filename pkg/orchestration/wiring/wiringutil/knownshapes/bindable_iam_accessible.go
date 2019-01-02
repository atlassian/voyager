package knownshapes

import "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"

const (
	// BindableIamAccessibleShape is a When you bind to it it returns IAM policy snippet.
	BindableIamAccessibleShape wiringplugin.ShapeName = "voyager.atl-paas.net/BindableIamAccessible"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type BindableIamAccessible struct {
	wiringplugin.ShapeMeta           `json:",inline"`
	wiringplugin.BindableShapeStruct `json:",inline"`
	//IAMRoleARN    BindingProtoReference
	//IAMProfileARN BindingProtoReference
	IAMPolicySnippet wiringplugin.BindingProtoReference
}

func (s *BindableIamAccessible) Name() wiringplugin.ShapeName {
	return BindableIamAccessibleShape
}
