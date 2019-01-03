package ops

import (
	"github.com/atlassian/voyager/pkg/apis/ops"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RouteCrd() *apiext_v1b1.CustomResourceDefinition {
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: ops_v1.RouteResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group: ops.GroupName,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:     ops_v1.RouteResourcePlural,
				Singular:   ops_v1.RouteResourceSingular,
				Kind:       ops_v1.RouteResourceKind,
				ShortNames: []string{ops_v1.RouteResourceShortName},
			},
			Scope: apiext_v1b1.NamespaceScoped,
			Validation: &apiext_v1b1.CustomResourceValidation{
				OpenAPIV3Schema: &apiext_v1b1.JSONSchemaProps{
					Properties: map[string]apiext_v1b1.JSONSchemaProps{
						"spec": {
							Type:     "object",
							Required: []string{"url", "asap"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"url": {
									Type:      "string",
									MaxLength: int64ptr(255),
									Pattern:   `^https?://.*$`,
								},
								"asap": {
									Type:     "object",
									Required: []string{"audience"},
									Properties: map[string]apiext_v1b1.JSONSchemaProps{
										"audience": {
											Type:      "string",
											MinLength: int64ptr(1),
										},
									},
								},
								"plans": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Type:      "string",
											MinLength: int64ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
			Versions: []apiext_v1b1.CustomResourceDefinitionVersion{
				{
					Name:    ops_v1.RouteResourceVersion,
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
