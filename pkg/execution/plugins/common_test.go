package plugins

import (
	"testing"

	plugin_testing "github.com/atlassian/voyager/pkg/execution/plugins/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestFindSecretPresent(t *testing.T) {
	t.Parallel()
	secret1 := map[string][]byte{"a": []byte("b")}
	secret2 := map[string][]byte{"c": []byte("d")}
	secret2b := map[string][]byte{"e": []byte("f")}
	outputs := []runtime.Object{
		plugin_testing.ConstructSecret("secret1", "ns", secret1),
		plugin_testing.ConstructSecret("secret2", "ns1", secret2),
		plugin_testing.ConstructSecret("secret2", "ns", secret2b),
	}

	binding := plugin_testing.ConstructBinding("binding", "ns", "secret2", "instance")

	foundSecret := FindBindingSecret(binding, outputs)
	require.NotNil(t, foundSecret)
	assert.Equal(t, secret2b, foundSecret.Data)
}

func TestFindSecretMissing(t *testing.T) {
	t.Parallel()
	secret1 := map[string][]byte{"a": []byte("b")}
	outputs := []runtime.Object{
		plugin_testing.ConstructSecret("secret1", "ns", secret1),
	}

	binding := plugin_testing.ConstructBinding("binding", "ns", "secret2", "instance")

	foundSecret := FindBindingSecret(binding, outputs)
	assert.Nil(t, foundSecret)
}

func TestFindServiceInstancePresent(t *testing.T) {
	t.Parallel()
	auxiliaryObjects := []runtime.Object{
		plugin_testing.ConstructInstance("instance2", "ns", "default"),
		plugin_testing.ConstructInstance("instance", "ns", "default"),
	}
	binding := plugin_testing.ConstructBinding("binding", "ns", "secret2", "instance")
	instance := FindServiceInstance(binding, auxiliaryObjects)
	require.NotNil(t, instance)
	assert.Equal(t, auxiliaryObjects[1], instance)
}

func TestFindServiceInstanceMissing(t *testing.T) {
	t.Parallel()
	auxiliaryObjects := []runtime.Object{
		plugin_testing.ConstructInstance("instance2", "ns", "default"),
		plugin_testing.ConstructInstance("instance3", "ns", "default"),
	}
	binding := plugin_testing.ConstructBinding("binding", "ns", "secret2", "instance")
	instance := FindServiceInstance(binding, auxiliaryObjects)
	assert.Nil(t, instance)
}
