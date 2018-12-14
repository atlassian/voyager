package svccatentangler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestInstanceId(t *testing.T) {
	t.Parallel()

	expected := "washere"
	actual, err := InstanceID(
		&runtime.RawExtension{
			Raw: []byte(`{"instanceId":"` + expected + `"}`),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}
