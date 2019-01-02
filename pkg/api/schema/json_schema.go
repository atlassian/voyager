package schema

import (
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

const (
	ResourceNameSchemaPattern = `^[a-z0-9]((-?[a-z0-9])*)?(\.[a-z0-9]((-?[a-z0-9])*)?)*$`
)

// ResourceNameSchema returns the JSON schema for a resource name (voyager.ResourceName).
func ResourceNameSchema() apiext_v1b1.JSONSchemaProps {
	// resourceName is based off DNS_SUBDOMAIN, except we don't allow double
	// dash in the name.
	return apiext_v1b1.JSONSchemaProps{
		Type:      "string",
		MinLength: int64ptr(1),
		MaxLength: int64ptr(253),
		Pattern:   ResourceNameSchemaPattern,
	}
}

func int64ptr(val int64) *int64 {
	return &val
}
