package schematest

import (
	"testing"

	"github.com/go-openapi/validate"
	"github.com/stretchr/testify/require"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiservervalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
)

func SchemaValidatorForCRD(t *testing.T, crd *apiext_v1b1.CustomResourceDefinition) *validate.SchemaValidator {
	crValidation := apiextensions.CustomResourceValidation{}
	err := apiext_v1b1.Convert_v1beta1_CustomResourceValidation_To_apiextensions_CustomResourceValidation(crd.Spec.Validation, &crValidation, nil)
	require.NoError(t, err)
	validator, _, err := apiservervalidation.NewSchemaValidator(&crValidation)
	require.NoError(t, err)
	return validator
}
