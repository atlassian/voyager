package testing

import (
	"reflect"
	"testing"

	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShape(t *testing.T, shape wiringplugin.Shape) {
	t.Run("type is a pointer", func(t *testing.T) {
		st := reflect.TypeOf(shape)
		assert.Equal(t, reflect.Ptr, st.Kind())
	})

	t.Run("round-trip via unstructured", func(t *testing.T) {
		var unstructured wiringplugin.UnstructuredShape

		require.NoError(t, wiringplugin.CopyShape(shape, &unstructured))

		st := reflect.TypeOf(shape).Elem()
		newShape := reflect.New(st).Interface().(wiringplugin.Shape)

		require.NoError(t, wiringplugin.CopyShape(&unstructured, newShape))
		assert.Equal(t, shape, newShape)
	})

	t.Run("copy", func(t *testing.T) {
		st := reflect.TypeOf(shape).Elem()
		newShape := reflect.New(st).Interface().(wiringplugin.Shape)

		require.NoError(t, wiringplugin.CopyShape(shape, newShape))
		assert.Equal(t, shape, newShape)
	})

	t.Run("deep copy", func(t *testing.T) {
		assert.Equal(t, shape, shape.DeepCopyShape())
	})
}
