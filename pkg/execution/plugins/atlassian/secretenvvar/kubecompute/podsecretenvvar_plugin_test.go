package kubecompute

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
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

func testEnvVarsFull(t *testing.T, renameEnvVar map[string]string, ignoreKeyRegex string, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency, expectedResult map[string]string) {
	p, err := New()
	require.NoError(t, err)

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.PodSpec{
		RenameEnvVar:   renameEnvVar,
		IgnoreKeyRegex: ignoreKeyRegex,
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace:    defaultNamespace,
		Dependencies: dependencies,
	}

	result, err := p.Process(spec, context)
	require.NoError(t, err)
	secret := result.Object.(*core_v1.Secret)

	for expectedKey, expectedVal := range expectedResult {
		actualVal, ok := secret.Data[expectedKey]
		require.True(t, ok, "missing output secret key: %q", expectedKey)
		assert.Equal(t, expectedVal, string(actualVal))
	}
	assert.Equal(t, len(expectedResult), len(secret.Data))
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

func TestRenameAsapKey(t *testing.T) {
	t.Parallel()
	asapCredentials := map[string][]byte{
		"AUDIENCE":    []byte("audience"),
		"ISSUER":      []byte("issuer"),
		"KEY_ID":      []byte("keyId"),
		"PRIVATE_KEY": []byte("privateKey"),
	}
	expectedResult := map[string]string{
		"ASAP_AUDIENCE":    "audience",
		"ASAP_ISSUER":      "issuer",
		"ASAP_KEY_ID":      "keyId",
		"ASAP_PRIVATE_KEY": "privateKey",
	}
	dependencies := map[smith_v1.ResourceName]smith_plugin.Dependency{
		"x": plugin_testing.ConstructBindingDependency(
			"asap-binding",
			defaultNamespace,
			"asap-secret",
			"myasap",
			"asap",
			asapCredentials),
	}
	dependencies["x"].Auxiliary[0].(*sc_v1b1.ServiceInstance).Annotations = map[string]string{
		"voyager.atl-paas.net/envResourcePrefix": "ASAPKey",
	}
	testEnvVarsFull(t, map[string]string{}, "", dependencies, expectedResult)
}
