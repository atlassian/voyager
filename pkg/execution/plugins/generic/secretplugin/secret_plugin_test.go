package secretplugin

import (
	"testing"

	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
)

func TestProcessJSONData(t *testing.T) {
	t.Parallel()

	plugin, err := New()
	require.NoError(t, err)

	rawSpec := map[string]interface{}{
		"jsondata": map[string]interface{}{
			"k1": "data",
			"k2": map[string]interface{}{
				"other": "data",
			},
		},
	}

	context := smith_plugin.Context{}

	result := plugin.Process(rawSpec, &context)
	require.Equal(t, smith_plugin.ProcessResultSuccessType, result.StatusType())
	secret := result.(*smith_plugin.ProcessResultSuccess).Object.(*core_v1.Secret)

	assert.Equal(t, "\"data\"", string(secret.Data["k1"]))
	assert.Equal(t, "{\"other\":\"data\"}", string(secret.Data["k2"]))
}

func TestProcessData(t *testing.T) {
	t.Parallel()

	plugin, err := New()
	require.NoError(t, err)

	rawSpec := map[string]interface{}{
		"data": map[string]string{
			"k1": "data",
		},
	}

	context := smith_plugin.Context{}

	result := plugin.Process(rawSpec, &context)
	require.Equal(t, smith_plugin.ProcessResultSuccessType, result.StatusType())
	secret := result.(*smith_plugin.ProcessResultSuccess).Object.(*core_v1.Secret)

	assert.Equal(t, "data", string(secret.Data["k1"]))
}
