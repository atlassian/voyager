package compute

import (
	"fmt"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenameEnvironmentVariables(t *testing.T) {
	t.Parallel()

	// given
	renameMap := map[string]string{
		"SQS_QUEUE1_A_B_C": "SECRET1_MYSECRET",
		"SECRET1_MYSECRET": "SQS_QUEUE1_A_B_C",
	}
	environmentVariables := map[string]string{
		"SECRET1_MYSECRET": "1",
		"SQS_QUEUE1_A_B_C": "val1",
		"SQS_QUEUE2_A_B_C": "val2",
	}

	// expected results
	expectedResult := map[string]string{
		"SECRET1_MYSECRET": "val1",
		"SQS_QUEUE1_A_B_C": "1",
		"SQS_QUEUE2_A_B_C": "val2",
	}

	// do
	renamed, err := renameEnvironmentVariables(renameMap, environmentVariables)
	require.NoError(t, err)

	// compare
	assert.Equal(t, expectedResult, renamed)
}

func TestRenameEnvironmentVariablesNonExisting(t *testing.T) {
	t.Parallel()

	// given
	renameMap := map[string]string{
		"ANOTHER_ENV_1": "WHAT_IS_THIS_EVEN",
	}
	environmentVariables := map[string]string{
		"SECRET1_MYSECRET": "1",
		"SQS_QUEUE1_A_B_C": "val1",
		"SQS_QUEUE2_A_B_C": "val2",
	}

	// do
	_, err := renameEnvironmentVariables(renameMap, environmentVariables)
	require.Error(t, err)
}

func TestRenameEnvironmentVariablesDuplicate(t *testing.T) {
	t.Parallel()

	// given
	renameMap := map[string]string{
		"SQS_QUEUE2_A_B_C": "SQS_QUEUE1_A_B_C",
	}
	environmentVariables := map[string]string{
		"SECRET1_MYSECRET": "1",
		"SQS_QUEUE1_A_B_C": "val1",
		"SQS_QUEUE2_A_B_C": "val2",
	}

	// do
	_, err := renameEnvironmentVariables(renameMap, environmentVariables)
	require.Error(t, err)
}

func TestGenerateEnvVars(t *testing.T) {
	t.Parallel()

	// given
	// - a rename map that renames a single variable
	// - a binding for a queue
	// - a binding for a database
	renameMap := map[string]string{
		"SQS_MY_QUEUE_A_B_C": "SQS_QUEUE1_A_B_C",
	}
	queueBinding := constructResourceWithEnvBinding("my-compute", "my-queue", "my-queue--instance", "SQS", map[string]string{
		"A_B_C":        "data.abc[0]",
		"A_B_C_2":      "data.abc[1]",
		"ABC":          "data.abc[2]",
		"MY_OTHER_VAR": "data.myOtherVar",
	})
	databaseBinding := constructResourceWithEnvBinding("my-compute", "my-db", "my-db--instance", "PG", map[string]string{
		"USERNAME": "data.username",
	})

	// expected
	// - The reference here will be used by the ec2compute to refer to a secret owned
	//   by the binding. The binding's name in this case is `{consumer}--{producer}--binding`,
	//   and the generated reference name appends the hash to the end of the binding.
	// - There is one reference for each secret.
	expectedReference1 := smith_v1.Reference{
		Name:     smith_v1.ReferenceName("my-compute--my-queue--binding-" + makeRefPathSuffix("data.abc[0]")),
		Resource: "my-compute--my-queue--binding",
		Path:     "data.abc[0]",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	expectedReference2 := smith_v1.Reference{
		Name:     smith_v1.ReferenceName("my-compute--my-queue--binding-" + makeRefPathSuffix("data.abc[1]")),
		Resource: "my-compute--my-queue--binding",
		Path:     "data.abc[1]",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	expectedReference3 := smith_v1.Reference{
		Name:     smith_v1.ReferenceName("my-compute--my-queue--binding-" + makeRefPathSuffix("data.abc[2]")),
		Resource: "my-compute--my-queue--binding",
		Path:     "data.abc[2]",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	expectedReference4 := smith_v1.Reference{
		Name:     smith_v1.ReferenceName("my-compute--my-queue--binding-" + makeRefPathSuffix("data.myOtherVar")),
		Resource: "my-compute--my-queue--binding",
		Path:     "data.myOtherVar",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	expectedReference5 := smith_v1.Reference{
		Name:     smith_v1.ReferenceName("my-compute--my-db--binding-" + makeRefPathSuffix("data.username")),
		Resource: "my-compute--my-db--binding",
		Path:     "data.username",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	expectedEnvVars := map[string]string{
		"SQS_QUEUE1_A_B_C":          fmt.Sprintf("!{my-compute--my-queue--binding-%s}", makeRefPathSuffix("data.abc[0]")),
		"SQS_MY_QUEUE_A_B_C_2":      fmt.Sprintf("!{my-compute--my-queue--binding-%s}", makeRefPathSuffix("data.abc[1]")),
		"SQS_MY_QUEUE_ABC":          fmt.Sprintf("!{my-compute--my-queue--binding-%s}", makeRefPathSuffix("data.abc[2]")),
		"SQS_MY_QUEUE_MY_OTHER_VAR": fmt.Sprintf("!{my-compute--my-queue--binding-%s}", makeRefPathSuffix("data.myOtherVar")),
		"PG_MY_DB_USERNAME":         fmt.Sprintf("!{my-compute--my-db--binding-%s}", makeRefPathSuffix("data.username")),
	}

	// do
	smithReferences, envVars, err := GenerateEnvVars(renameMap, []ResourceWithEnvVarBinding{queueBinding, databaseBinding})
	require.NoError(t, err)

	// results
	require.Len(t, smithReferences, 5)
	assert.Contains(t, smithReferences, expectedReference1)
	assert.Contains(t, smithReferences, expectedReference2)
	assert.Contains(t, smithReferences, expectedReference3)
	assert.Contains(t, smithReferences, expectedReference4)
	assert.Contains(t, smithReferences, expectedReference5)
	assert.Equal(t, expectedEnvVars, envVars)
}

func TestGenerateEnvVarsEmptyBindings(t *testing.T) {
	t.Parallel()

	// Does nothing if empty - this implies there were no dependencies, thus there are no environment variables
	smithReferences, envVars, err := GenerateEnvVars(map[string]string{}, []ResourceWithEnvVarBinding{})
	require.NoError(t, err)
	assert.Len(t, smithReferences, 0)
	assert.Len(t, envVars, 0)
}

func TestGenerateEnvVarsEmptyVars(t *testing.T) {
	t.Parallel()

	// given
	queueBinding := constructResourceWithEnvBinding("my-compute", "my-queue", "my-queue--instance", "SQS", map[string]string{})

	// do
	smithReferences, envVars, err := GenerateEnvVars(map[string]string{}, []ResourceWithEnvVarBinding{queueBinding})
	require.NoError(t, err)

	// results
	assert.Len(t, smithReferences, 0)
	assert.Len(t, envVars, 0)
}

func constructResourceWithEnvBinding(consumerName string, dependencyName voyager.ResourceName, serviceInstance smith_v1.ResourceName, prefix string, vars map[string]string) ResourceWithEnvVarBinding {
	return ResourceWithEnvVarBinding{
		// This binding result describes a dependency to a resource in the State
		ResourceName: dependencyName,

		// The shape is the shape output by that particular resource
		// In this case, it's always a BindableEnvVar shape.
		BindableEnvVarShape: *knownshapes.NewBindableEnvironmentVariables(serviceInstance, prefix, vars),

		// This describes the binding to the serviceinstance described in the shape above
		// As per convention, the naming is {consumer}--{producer}--binding - this is actually
		// generated by the EC2Compute so this test may become out of sync, but we don't test
		// the naming here so it's fine.
		// - The consumer is usually something like an ec2compute (name of the ec2compute)
		// - The producer is the dependency (the name of the SQS, SNS, etc)
		CreatedBindingFromShape: smith_v1.Resource{
			Name: smith_v1.ResourceName(fmt.Sprintf("%s--%s--binding", consumerName, dependencyName)),
			// This usually contains references but we don't care about the spec here
			References: []smith_v1.Reference{},
			// Spec is ignored for tests - we only care about the generated references and envvars
			Spec: smith_v1.ResourceSpec{},
		},
	}
}
