package secretparameter

import (
	"encoding/json"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	plugin_testing "github.com/atlassian/voyager/pkg/execution/plugins/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultNamespace = "ns"
)

// Should never happen (schema blocked)
func TestMissingMapping(t *testing.T) {
	t.Parallel()

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&Spec{})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace: defaultNamespace,
	}

	p, err := New()
	require.NoError(t, err)
	result := p.Process(spec, context)
	require.Equal(t, smith_plugin.ProcessResultFailureType, result.StatusType())
	require.EqualError(t, result.(*smith_plugin.ProcessResultFailure).Error, "spec is invalid - must provide at least one secret to map")
}

func TestBasic(t *testing.T) {
	t.Parallel()
	input1 := map[string][]byte{
		"dnsName":       []byte("google.com"),
		"someotherdata": []byte("foo"),
		"somemoredata":  []byte("bar"),
	}
	input2 := map[string][]byte{
		"dnsName": []byte("yahoo.com"),
	}

	spec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&Spec{
		Mapping: map[smith_v1.ResourceName]map[string]string{
			"binding1": {
				"dnsName":       "primaryCompute",
				"someotherdata": "extra",
			},
			"binding2": {
				"dnsName": "secondaryCompute",
			},
			"secret1": {
				"mysecret": "myFakeSecret",
			},
		},
	})
	require.NoError(t, err)

	context := &smith_plugin.Context{
		Namespace: defaultNamespace,
		Dependencies: map[smith_v1.ResourceName]smith_plugin.Dependency{
			"binding1": plugin_testing.ConstructBindingDependency("binding1", defaultNamespace, "secret1", "compute1", "compute", input1),
			"binding2": plugin_testing.ConstructBindingDependency("binding2", defaultNamespace, "secret2", "queue2", "sqs", input2),
			"secret1":  plugin_testing.ConstructSecretDependency("secret1", defaultNamespace, map[string][]byte{"mysecret": []byte("1")}),
		},
	}

	p, err := New()
	require.NoError(t, err)
	result := p.Process(spec, context)
	require.Equal(t, smith_plugin.ProcessResultSuccessType, result.StatusType())
	secret := result.(*smith_plugin.ProcessResultSuccess).Object.(*core_v1.Secret)

	assertSecretKey(t, secret.Data["binding1"], map[string]string{
		"primaryCompute": "google.com",
		"extra":          "foo",
	})

	assertSecretKey(t, secret.Data["binding2"], map[string]string{
		"secondaryCompute": "yahoo.com",
	})

	assertSecretKey(t, secret.Data["secret1"], map[string]string{
		"myFakeSecret": "1",
	})
}

func assertSecretKey(t *testing.T, data []byte, expectedParams map[string]string) {
	require.NotNil(t, data, "missing output secret key")

	var actualParams map[string]string
	err := json.Unmarshal(data, &actualParams)
	require.NoError(t, err)
	assert.Equal(t, expectedParams, actualParams)
}
