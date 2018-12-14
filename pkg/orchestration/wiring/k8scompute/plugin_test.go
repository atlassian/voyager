package k8scompute

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWireUP(t *testing.T) {
	t.Parallel()

	t.Run("spec validator should not throw any error", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: "image:2.3"}, {Name: "container2", Image: "image@sha256:f1ab25acfc1331f58cb2c2d4f93eec7e52c59475f2831bd05cd715d5df9cfbbf"}}
		err := validateContainerDockerImage(&spec)
		require.NoError(t, err)
	})

	t.Run("spec validator should throw error for not specifying the tag", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: "image"}, {Name: "container2", Image: "image:2389"}}
		err := validateContainerDockerImage(&spec)
		require.Error(t, err)
	})

	t.Run("spec validator should throw error for empty tag", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: "image:"}}
		err := validateContainerDockerImage(&spec)
		require.Error(t, err)
	})

	t.Run("spec validator should throw error for empty digest", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: "image@"}}
		err := validateContainerDockerImage(&spec)
		require.Error(t, err)
	})

	t.Run("spec validator should throw error for empty image name with tag", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: ":image"}}
		err := validateContainerDockerImage(&spec)
		require.Error(t, err)
	})

	t.Run("spec validator should throw error for empty image name with digest", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "container1", Image: "@image"}}
		err := validateContainerDockerImage(&spec)
		require.Error(t, err)
	})
}
