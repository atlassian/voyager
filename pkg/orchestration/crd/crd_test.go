package orchestration

import (
	"testing"

	"github.com/atlassian/voyager/pkg/api/schema/schematest"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/go-openapi/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
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

func TestStateSchema(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		errorMsg string
	}{
		{
			"test_crd_requires_configmapname",
			"spec.configMapName in body is required",
		},
		{
			"test_crd_does_not_require_resources",
			"",
		},
	}

	schemaValidator := schematest.SchemaValidatorForCRD(t, StateCrd())

	for _, tc := range cases {
		filename := tc.name + ".yml" // files are all yamls files
		t.Run(filename, func(t *testing.T) {
			runCRDTestCase(t, schemaValidator, filename, tc.errorMsg)
		})
	}

}
