package composition

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/composition"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceDescriptorCrd provides the custom resource definition for a ServiceDescriptor
func ServiceDescriptorCrd() *apiext_v1b1.CustomResourceDefinition {
	locations := apiext_v1b1.JSONSchemaProps{
		Type: "array",
		Items: &apiext_v1b1.JSONSchemaPropsOrArray{
			Schema: &apiext_v1b1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiext_v1b1.JSONSchemaProps{
					"name": {
						Type: "string",
					},
					"account": {
						Type: "string",
					},
					"region": {
						Type: "string",
					},
					"envType": {
						Type: "string",
					},
					"label": {
						Type: "string",
					},
				},
				Required: []string{"name", "region", "envType"},
			},
		},
	}

	config := apiext_v1b1.JSONSchemaProps{
		Type: "array",
		Items: &apiext_v1b1.JSONSchemaPropsOrArray{
			Schema: &apiext_v1b1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiext_v1b1.JSONSchemaProps{
					"scope": {
						Type:      "string",
						MinLength: int64ptr(1),
						MaxLength: int64ptr(253),
						// force to either global or dev|staging|prod
						// with optional ordered qualifiers of aws region, label, and account number
						Pattern: `^(global|(dev|staging|prod)(\.[a-z]{2}-[a-z]{3,20}-\d(\.[a-z0-9\-]*(\.\d{12})?)?)?)$`,
					},
					"vars": {
						Type:                 "object",
						AdditionalProperties: &apiext_v1b1.JSONSchemaPropsOrBool{Allows: true},
					},
				},
				Required: []string{"scope", "vars"},
			},
		},
	}

	version := apiext_v1b1.JSONSchemaProps{
		Type: "string", // can't provide a default in k8s schema yet
	}

	resourceName := voyager.ResourceNameSchema()
	resourceGroups := apiext_v1b1.JSONSchemaProps{
		Type:     "array",
		MinItems: int64ptr(1),
		Items: &apiext_v1b1.JSONSchemaPropsOrArray{
			Schema: &apiext_v1b1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiext_v1b1.JSONSchemaProps{
					"name": {
						Type: "string",
					},
					"locations": {
						Type:     "array",
						MinItems: int64ptr(1),
						Items: &apiext_v1b1.JSONSchemaPropsOrArray{
							Schema: &apiext_v1b1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					"resources": {
						Type: "array",
						Items: &apiext_v1b1.JSONSchemaPropsOrArray{
							Schema: &apiext_v1b1.JSONSchemaProps{
								Type:     "object",
								Required: []string{"name", "type"},
								Properties: map[string]apiext_v1b1.JSONSchemaProps{
									"name": resourceName,
									"type": {
										Type: "string",
									},
									"spec": {
										Type:                 "object",
										AdditionalProperties: &apiext_v1b1.JSONSchemaPropsOrBool{Allows: true},
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
								},
							},
						},
						MinItems: int64ptr(0),
					},
				},
				Required: []string{"name", "locations", "resources"},
			},
		},
	}

	return &apiext_v1b1.CustomResourceDefinition{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: apiext_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: comp_v1.ServiceDescriptorResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group: composition.GroupName,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:     comp_v1.ServiceDescriptorResourcePlural,
				Singular:   comp_v1.ServiceDescriptorResourceSingular,
				Kind:       comp_v1.ServiceDescriptorResourceKind,
				ListKind:   comp_v1.ServiceDescriptorResourceListKind,
				ShortNames: []string{"sd"},
			},
			Scope: apiext_v1b1.ClusterScoped,
			Validation: &apiext_v1b1.CustomResourceValidation{
				OpenAPIV3Schema: &apiext_v1b1.JSONSchemaProps{
					Properties: map[string]apiext_v1b1.JSONSchemaProps{
						"spec": {
							Type:     "object",
							Required: []string{"locations"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"locations":      locations,
								"config":         config,
								"resourceGroups": resourceGroups,
								"version":        version,
							},
						},
					},
				},
			},
			Versions: []apiext_v1b1.CustomResourceDefinitionVersion{
				{
					Name:    comp_v1.ServiceDescriptorResourceVersion,
					Served:  true,
					Storage: true,
				},
			},
		}}
}

func int64ptr(val int64) *int64 {
	return &val
}
