package crd

import (
	"testing"

	"github.com/atlassian/voyager/pkg/api/schema/schematest"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/ghodss/yaml"
	"github.com/go-openapi/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCRDTestCase(t *testing.T, schemaValidator *validate.SchemaValidator, filename string, errorMsg string) {
	srcFile, err := testutil.LoadFileFromTestData(filename)
	require.NoError(t, err)

	var testData interface{}
	err = yaml.Unmarshal(srcFile, &testData)
	require.NoError(t, err)

	result := schemaValidator.Validate(testData)

	if errorMsg == "" {
		assert.NoError(t, result.AsError())
		return
	}

	require.Error(t, result.AsError())
	assert.EqualError(t, result.Errors[0], errorMsg)
}

func TestServiceDescriptorSchema(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		errorMsg string
	}{
		{
			"test_crd_requires_resource_group_locations",
			"spec.resourceGroups.locations in body is required",
		},
		{
			"test_crd_allows_no_resource_group_resources",
			"",
		},
		{
			"test_crd_allows_scope_with_account_no_label",
			"",
		},
	}

	schemaValidator := schematest.SchemaValidatorForCRD(t, ServiceDescriptorCrd())

	for _, tc := range cases {
		filename := tc.name + ".yml" // files are all yaml files
		t.Run(filename, func(t *testing.T) {
			runCRDTestCase(t, schemaValidator, filename, tc.errorMsg)
		})
	}

}
