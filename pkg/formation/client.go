package formation

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/formation"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func LocationDescriptorCrd() *apiext_v1b1.CustomResourceDefinition {
	// NB: Currently this is an exact copy/paste of the orchestration/state CRD with Defaults removed

	// Schema is based on:
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/namespaces.md
	// https://github.com/kubernetes/kubernetes/tree/master/api/openapi-spec

	// definitions are not supported, do what we can :)

	resourceName := voyager.ResourceNameSchema()
	resource := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name", "type"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": resourceName,
			"type": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"dependsOn": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &apiext_v1b1.JSONSchemaProps{
						OneOf: []apiext_v1b1.JSONSchemaProps{
							resourceName,
							{
								Properties: map[string]apiext_v1b1.JSONSchemaProps{
									"name": resourceName,
									"attributes": {
										Type: "object",
									},
								},
								Required: []string{"name"},
								Type:     "object",
							},
						},
					},
				},
			},
			"spec": {
				Type: "object",
			},
		},
	}
	return &apiext_v1b1.CustomResourceDefinition{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: apiext_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: form_v1.LocationDescriptorResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group: formation.GroupName,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:     form_v1.LocationDescriptorResourcePlural,
				Singular:   form_v1.LocationDescriptorResourceSingular,
				Kind:       form_v1.LocationDescriptorResourceKind,
				ShortNames: []string{"ld"},
			},
			Scope: apiext_v1b1.NamespaceScoped,
			Validation: &apiext_v1b1.CustomResourceValidation{
				OpenAPIV3Schema: &apiext_v1b1.JSONSchemaProps{
					Properties: map[string]apiext_v1b1.JSONSchemaProps{
						"spec": {
							Type: "object",
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"resources": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &resource,
									},
								},
							},
						},
					},
				},
			},
			Versions: []apiext_v1b1.CustomResourceDefinitionVersion{
				{
					Name:    form_v1.LocationDescriptorResourceVersion,
					Served:  true,
					Storage: true,
				},
			},
		},
	}

}

func int64ptr(val int64) *int64 {
	return &val
}
