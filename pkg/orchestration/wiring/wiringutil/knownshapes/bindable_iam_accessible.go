package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
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
	libshapes.BindableShapeStruct `json:",inline"`
	//IAMRoleARN    BindingProtoReference
	//IAMProfileARN BindingProtoReference
	IAMPolicySnippet libshapes.BindingSecretProtoReference
}

func NewBindableIamAccessible(resourceName smith_v1.ResourceName, iamPOlicySnippetPath string) *BindableIamAccessible {
	return &BindableIamAccessible{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: BindableIamAccessibleShape,
		},
		Data: BindableIamAccessibleData{
			BindableShapeStruct: libshapes.BindableShapeStruct{
				ServiceInstanceName: libshapes.ProtoReference{
					Resource: resourceName,
					Path:     "metadata.name",
					Example:  "aname",
				}},
			IAMPolicySnippet: libshapes.BindingSecretProtoReference{
				Path: iamPOlicySnippetPath,
			},
		},
	}
}

func FindBindableIamAccessibleShape(shapes []wiringplugin.Shape) (*BindableIamAccessible, bool /*found*/, error) {
	typed := &BindableIamAccessible{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, BindableIamAccessibleShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
