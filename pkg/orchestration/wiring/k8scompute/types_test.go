package k8scompute

import (
	"testing"

	"github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	t.Run("default scaling", func(t *testing.T) {
		spec := Spec{}

		// Defaults are passed in from the formation layer in the state
		state := v1.StateResource{
			Defaults: &runtime.RawExtension{Raw: []byte(`{"Scaling":{"MaxReplicas":5,"Metrics":[{"Resource":{"Name":"cpu","TargetAverageUtilization":80},"Type":"Resource"}],"MinReplicas":1}}`)},
		}
		require.NoError(t, spec.ApplyDefaults(state.Defaults))

		assert.Equal(t, int32(1), spec.Scaling.MinReplicas)
		assert.Equal(t, int32(5), spec.Scaling.MaxReplicas)

		assert.Len(t, spec.Scaling.Metrics, 1)
		assert.Equal(t, autoscaling_v2b1.MetricSourceType("Resource"), spec.Scaling.Metrics[0].Type)
		assert.Equal(t, core_v1.ResourceName("cpu"), spec.Scaling.Metrics[0].Resource.Name)
		assert.Equal(t, int32(80), *spec.Scaling.Metrics[0].Resource.TargetAverageUtilization)
	})

	t.Run("no scaling", func(t *testing.T) {
		spec := Spec{}

		state := v1.StateResource{
			Defaults: &runtime.RawExtension{Raw: []byte(`{}`)},
		}
		require.NoError(t, spec.ApplyDefaults(state.Defaults))

		assert.Equal(t, int32(0), spec.Scaling.MinReplicas)
		assert.Equal(t, int32(0), spec.Scaling.MaxReplicas)
		assert.Empty(t, spec.Scaling.Metrics)
	})

	t.Run("default ImagePullPolicy", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "Web server"}}

		state := v1.StateResource{
			Defaults: &runtime.RawExtension{Raw: []byte(`{"Container":{"ImagePullPolicy":"IfNotPresent"}}`)},
		}
		require.NoError(t, spec.ApplyDefaults(state.Defaults))

		container := spec.Containers[0]
		assert.Equal(t, "IfNotPresent", container.ImagePullPolicy)
	})

	t.Run("default Protocol", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "Web server", Ports: []ContainerPort{{ContainerPort: 8080}}}}

		state := v1.StateResource{
			Defaults: &runtime.RawExtension{Raw: []byte(`{"Port":{"Protocol":"TCP"}}`)},
		}
		require.NoError(t, spec.ApplyDefaults(state.Defaults))

		port := spec.Containers[0].Ports[0]
		assert.Equal(t, "TCP", port.Protocol)
	})

	t.Run("default resources", func(t *testing.T) {
		spec := Spec{}
		spec.Containers = []Container{{Name: "Web server"}}

		state := v1.StateResource{
			Defaults: &runtime.RawExtension{Raw: []byte(`{"Container":{"Resources":{"Limits":{"cpu":"250m","memory":"150Mi"},"Requests":{"cpu":"50m","memory":"50Mi"}}}}`)},
		}
		require.NoError(t, spec.ApplyDefaults(state.Defaults))

		resources := spec.Containers[0].Resources
		assert.Equal(t, resource.MustParse("250m"), resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("150Mi"), resources.Limits["memory"])
		assert.Equal(t, resource.MustParse("50m"), resources.Requests["cpu"])
		assert.Equal(t, resource.MustParse("50Mi"), resources.Requests["memory"])
	})
}

func TestToKubeContainer(t *testing.T) {
	t.Parallel()

	t.Run("Test ToKubeContainer", func(t *testing.T) {
		limits := ResourceList{"memory": resource.MustParse("128Mi")}
		requests := ResourceList{"memory": resource.MustParse("64Mi")}

		// setup
		container := Container{
			Name:            "container",
			Image:           "image",
			Command:         []string{"cmd"},
			Args:            []string{"abc"},
			WorkingDir:      ".",
			Ports:           []ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: "TCP"}},
			Env:             []EnvVar{{Name: "key", Value: "value"}},
			Resources:       ResourceRequirements{Limits: limits, Requests: requests},
			ImagePullPolicy: "Always",
		}

		expectedKubeContainer := core_v1.Container{
			Name:       "container",
			Image:      "image",
			Command:    []string{"cmd"},
			Args:       []string{"abc"},
			WorkingDir: ".",
			Ports:      []core_v1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: "TCP"}},
			Env:        []core_v1.EnvVar{{Name: "key", Value: "value"}},
			EnvFrom:    []core_v1.EnvFromSource{},
			Resources: core_v1.ResourceRequirements{
				Limits:   core_v1.ResourceList{"memory": resource.MustParse("128Mi")},
				Requests: core_v1.ResourceList{"memory": resource.MustParse("64Mi")},
			},
			ImagePullPolicy:          core_v1.PullAlways,
			TerminationMessagePath:   core_v1.TerminationMessagePathDefault,
			TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
		}

		// execute
		kubeContainer := container.ToKubeContainer([]core_v1.EnvVar{}, []core_v1.EnvFromSource{})

		// assert
		assert.Equal(t, expectedKubeContainer, kubeContainer)
	})

	t.Run("Test set EnvFrom", func(t *testing.T) {
		// setup
		container := Container{
			Name:  "container",
			Image: "image",
			EnvFrom: []EnvFromSource{
				{
					Prefix: "prefix1",
					ConfigMapRef: &ConfigMapEnvSource{
						LocalObjectReference: LocalObjectReference{
							Name: "configMapA",
						},
					},
				},
			},
		}

		expectedKubeContainer := core_v1.Container{
			Name:  "container",
			Image: "image",
			EnvFrom: []core_v1.EnvFromSource{
				{
					Prefix: "prefix3",
				},
				{
					Prefix:       "prefix1",
					ConfigMapRef: &core_v1.ConfigMapEnvSource{LocalObjectReference: core_v1.LocalObjectReference{Name: "configMapA"}},
				},
			},
		}

		// execute
		kubeContainer := container.ToKubeContainer([]core_v1.EnvVar{}, []core_v1.EnvFromSource{{Prefix: "prefix3"}})

		// assert
		assert.Equal(t, len(expectedKubeContainer.EnvFrom), len(kubeContainer.EnvFrom))
		for i := range expectedKubeContainer.EnvFrom {
			assert.Equal(t, expectedKubeContainer.EnvFrom[i], kubeContainer.EnvFrom[i])
		}
	})

	t.Run("Test set Env", func(t *testing.T) {
		// setup
		container := Container{
			Name:  "container",
			Image: "image",
			EnvFrom: []EnvFromSource{
				{
					Prefix: "prefix1",
					ConfigMapRef: &ConfigMapEnvSource{
						LocalObjectReference: LocalObjectReference{
							Name: "configMapA",
						},
					},
				},
			},
		}

		expectedKubeContainer := core_v1.Container{
			Name:  "container",
			Image: "image",
			Env: []core_v1.EnvVar{
				{
					Name:  "env-key",
					Value: "env-value",
				},
			},
			EnvFrom: []core_v1.EnvFromSource{
				{
					Prefix: "prefix3",
				},
				{
					Prefix:       "prefix1",
					ConfigMapRef: &core_v1.ConfigMapEnvSource{LocalObjectReference: core_v1.LocalObjectReference{Name: "configMapA"}},
				},
			},
		}

		// execute
		kubeContainer := container.ToKubeContainer([]core_v1.EnvVar{{Name: "key-env", Value: "env-value"}}, []core_v1.EnvFromSource{{Prefix: "prefix3"}})

		// assert
		assert.Equal(t, len(expectedKubeContainer.EnvFrom), len(kubeContainer.EnvFrom))
		for i := range expectedKubeContainer.EnvFrom {
			assert.Equal(t, expectedKubeContainer.EnvFrom[i], kubeContainer.EnvFrom[i])
		}
	})

	t.Run("Test set Env Merge", func(t *testing.T) {
		// setup
		container := Container{
			Name:  "container",
			Image: "image",
			Env: []EnvVar{
				{
					Name:  "key",
					Value: "value",
				},
			},
		}

		expectedKubeContainer := core_v1.Container{
			Name:  "container",
			Image: "image",
			Env: []core_v1.EnvVar{
				{
					Name:  "key",
					Value: "value",
				},
				{
					Name:  "default-key",
					Value: "default-value",
				},
			},
			EnvFrom: []core_v1.EnvFromSource{},
		}

		// execute
		kubeContainer := container.ToKubeContainer([]core_v1.EnvVar{{Name: "default-key", Value: "default-value"}}, []core_v1.EnvFromSource{})

		// assert
		assert.Equal(t, len(expectedKubeContainer.EnvFrom), len(kubeContainer.EnvFrom))
		for i := range expectedKubeContainer.EnvFrom {
			assert.Equal(t, expectedKubeContainer.EnvFrom[i], kubeContainer.EnvFrom[i])
		}
	})

}

func TestToKubeEnvVar(t *testing.T) {
	t.Parallel()

	t.Run("Name and Value", func(t *testing.T) {
		// setup
		envVar := EnvVar{
			Name:  "key",
			Value: "value",
		}

		expectedEnvVar := core_v1.EnvVar{
			Name:  "key",
			Value: "value",
		}

		// execute
		kubeEnvVar := envVar.toKubeEnvVar()

		// assert
		assert.Equal(t, expectedEnvVar, kubeEnvVar)
	})

	t.Run("ConfigMapKeyRef", func(t *testing.T) {
		// setup
		envVar := EnvVar{
			Name: "key",
			ValueFrom: &EnvVarSource{
				ConfigMapKeyRef: &ConfigMapKeySelector{
					LocalObjectReference: LocalObjectReference{
						Name: "configMap",
					},
					Key: "key",
				},
			},
		}

		expectedEnvVar := core_v1.EnvVar{
			Name: "key",
			ValueFrom: &core_v1.EnvVarSource{
				ConfigMapKeyRef: &core_v1.ConfigMapKeySelector{
					LocalObjectReference: core_v1.LocalObjectReference{
						Name: "configMap",
					},
					Key: "key",
				},
			},
		}

		// execute
		kubeEnvVar := envVar.toKubeEnvVar()

		// assert
		assert.Equal(t, expectedEnvVar, kubeEnvVar)
	})

}
