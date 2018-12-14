package microscompute

import (
	"encoding/json"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretenvvar"
	plugin_testing "github.com/atlassian/voyager/pkg/execution/plugins/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultNamespace = "ns"
)

func testEnvVars(t *testing.T, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency, expectedResult map[string]string) {
	testEnvVarsFull(t, map[string]string{}, "", dependencies, expectedResult)
}

func testEnvVarsFull(t *testing.T, renameEnvVar map[string]string, ignoreKeyRegex string,
	dependencies map[smith_v1.ResourceName]smith_plugin.Dependency, expectedResult map[string]string) {

	p, err := New()
	require.NoError(t, err)

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: "myenvvars",
		OutputJSONKey:   "secretEnvVars",
		RenameEnvVar:    renameEnvVar,
		IgnoreKeyRegex:  ignoreKeyRegex,
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace:    defaultNamespace,
		Dependencies: dependencies,
	}

	result, err := p.Process(spec, context)
	require.NoError(t, err)
	secret := result.Object.(*core_v1.Secret)

	myenvvars, ok := secret.Data["myenvvars"]
	require.True(t, ok, "missing output secret key")

	var actualEnv map[string]map[string]string
	err = json.Unmarshal(myenvvars, &actualEnv)
	require.NoError(t, err)
	assert.Equal(t, expectedResult, actualEnv["secretEnvVars"])
}

func TestNoDependencies(t *testing.T) {
	t.Parallel()

	testEnvVars(t, map[smith_v1.ResourceName]smith_plugin.Dependency{}, map[string]string{})
}

func TestBasic(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	expectedResult := map[string]string{
		"SECRET1_MYSECRET": "1",
		"SQS_QUEUE1_A_B_C": "val1",
		"SQS_QUEUE2_A_B_C": "val2",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"MYSECRET": []byte("1")}),
	}

	testEnvVars(t, dependencies, expectedResult)
}

func TestDashReplacement(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a0DASH0b0DASH0c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b0DASH0c": []byte("val2"),
	}
	expectedResult := map[string]string{
		"SQS_QUEUE1_A_B_C": "val1",
		"SQS_QUEUE2_A_B_C": "val2",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
	}

	testEnvVars(t, dependencies, expectedResult)
}

func TestAnnotationPrefixes(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	expectedResult := map[string]string{
		"MYSQS_QUEUE1_A_B_C":    "val1",
		"OTHERSQS_QUEUE2_A_B_C": "val2",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
	}
	dependencies["x"].Auxiliary[0].(*sc_v1b1.ServiceInstance).Annotations = map[string]string{
		"voyager.atl-paas.net/envResourcePrefix": "MYSQS",
	}
	dependencies["y"].Auxiliary[0].(*sc_v1b1.ServiceInstance).Annotations = map[string]string{
		"voyager.atl-paas.net/envResourcePrefix": "MYSQS",
	}
	dependencies["y"].Actual.(*sc_v1b1.ServiceBinding).Annotations = map[string]string{
		"voyager.atl-paas.net/envResourcePrefix": "OTHERSQS",
	}

	testEnvVars(t, dependencies, expectedResult)
}

// Should never happen (schema blocked)
func TestNoSecretKey(t *testing.T) {
	t.Parallel()

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputJSONKey: "myenvvars",
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace: defaultNamespace,
	}

	p, err := New()
	require.NoError(t, err)
	_, err = p.Process(spec, context)
	require.EqualError(t, err, "spec is invalid - must have both outputSecretKey and outputJsonKey")
}

// Should never happen (schema blocked)
func TestNoJsonKey(t *testing.T) {
	t.Parallel()

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: "myenvvars",
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace: defaultNamespace,
	}

	p, err := New()
	require.NoError(t, err)
	_, err = p.Process(spec, context)
	require.EqualError(t, err, "spec is invalid - must have both outputSecretKey and outputJsonKey")
}

func TestRenameEnvVars(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	expectedResult := map[string]string{
		"SECRET1_MYSECRET": "val1",
		"SQS_QUEUE1_A_B_C": "1",
		"SQS_QUEUE2_A_B_C": "val2",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"MYSECRET": []byte("1")}),
	}

	testEnvVarsFull(t, map[string]string{
		"SQS_QUEUE1_A_B_C": "SECRET1_MYSECRET",
		"SECRET1_MYSECRET": "SQS_QUEUE1_A_B_C",
	}, "", dependencies, expectedResult)
}

func TestRenameEnvVarsNonExisting(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"MYSECRET": []byte("1")}),
	}

	p, err := New()
	require.NoError(t, err)

	// This fails because not all keys can be renamed
	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: "myenvvars",
		OutputJSONKey:   "secretEnvVars",
		RenameEnvVar: map[string]string{
			"ANOTHER_ENV_1": "WHAT_IS_THIS_EVEN",
		},
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace:    defaultNamespace,
		Dependencies: dependencies,
	}

	_, err = p.Process(spec, context)
	require.Error(t, err)
}

func TestRenameEnvVarsDuplicate(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"MYSECRET": []byte("1")}),
	}

	p, err := New()
	require.NoError(t, err)

	// This fails because the key will clash
	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: "myenvvars",
		OutputJSONKey:   "secretEnvVars",
		RenameEnvVar: map[string]string{
			"SQS_QUEUE2_A_B_C": "SQS_QUEUE1_A_B_C",
		},
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace:    defaultNamespace,
		Dependencies: dependencies,
	}

	_, err = p.Process(spec, context)
	require.Error(t, err)
}

func TestIgnoreKeyRegex(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"a-b-c": []byte("val1"),
	}
	input2 := map[string][]byte{
		"a-b-c": []byte("val2"),
	}
	expectedResult := map[string]string{
		"SQS_QUEUE1_A_B_C": "val1",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "queue1", "sqs", input1),
		"y": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"MYSECRET": []byte("1")}),
	}

	testEnvVarsFull(t, map[string]string{}, "^S(ECRET1|QS_.*2)", dependencies, expectedResult)
}

func TestFilterAppliedAfterPrefix(t *testing.T) {
	t.Parallel()
	expectedResult := map[string]string{
		"SECRET1_A_B_C":                    "safe",
		"SECRET1_NOTQUITEIAMPOLICYSNIPPET": "safe",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"z": plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{
			"A_B_C":                    []byte("safe"),
			"IamPolicySnippet":         []byte("filterd"),
			"NotquiteIamPolicySnippet": []byte("safe"),
		}),
	}

	testEnvVarsFull(t, map[string]string{}, "^SECRET1_IAMPOLICYSNIPPET$", dependencies, expectedResult)
}
