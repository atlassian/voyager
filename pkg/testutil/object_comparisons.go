package testutil

import (
	"fmt"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_util "github.com/atlassian/smith/pkg/util"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func ObjectCompare(t *testing.T, resActual, resExpected runtime.Object) {
	ObjectCompareContext(t, FileName("expected"), resActual, resExpected)
}

func ObjectCompareContext(t *testing.T, fileName FileName, paramActual, paramExpected runtime.Object) {
	if paramActual == nil || paramExpected == nil {
		require.True(t, paramActual == paramExpected, "Mismatching nil: got actual %#v and expected %#v", paramActual, paramExpected)
		return
	}

	resActual := paramActual.DeepCopyObject()
	resExpected := paramExpected.DeepCopyObject()

	resActualUnstr, err := smith_util.RuntimeToUnstructured(resActual)
	require.NoError(t, err)

	resExpectedUnstr, err := smith_util.RuntimeToUnstructured(resExpected)
	require.NoError(t, err)

	YAMLCompareContext(t, fileName, resExpectedUnstr.Object, resActualUnstr.Object)
}

func PluginCompare(t *testing.T, resActual, resExpected *smith_v1.PluginSpec) {
	PluginCompareContext(t, FileName("expected"), resActual, resExpected)
}

func PluginCompareContext(t *testing.T, fileName FileName, paramActual, paramExpected *smith_v1.PluginSpec) {
	if paramActual == nil || paramExpected == nil {
		assert.True(t, paramActual == paramExpected, "Both smith plugins should be nil but got actual %#v and expected %#v", paramActual, paramExpected)
		return
	}

	resActual := paramActual.DeepCopy()
	resExpected := paramExpected.DeepCopy()

	expectedFileName := FileName(fmt.Sprintf("%s %s %s", fileName, resActual.Name, resActual.ObjectName))
	JSONCompareContext(t, expectedFileName, resExpected, resActual)
}

func ResourceCompare(t *testing.T, resActual, resExpected *smith_v1.Resource) {
	ResourceCompareContext(t, FileName("expected"), resActual, resExpected)
}

func ResourceCompareContext(t *testing.T, fileName FileName, paramActual, paramExpected *smith_v1.Resource) {
	resActual := paramActual.DeepCopy()
	resExpected := paramExpected.DeepCopy()

	resActual.Spec = smith_v1.ResourceSpec{}
	resExpected.Spec = smith_v1.ResourceSpec{}
	YAMLCompareContext(t, fileName, resExpected, resActual)
}

func BundleCompare(t *testing.T, bundleExpected, bundleActual *smith_v1.Bundle) {
	BundleCompareContext(t, FileName("expected"), bundleActual, bundleExpected)
}

func BundleCompareContext(t *testing.T, fileName FileName, paramExpected, paramActual *smith_v1.Bundle) {
	bundleActual := paramActual.DeepCopy()
	bundleExpected := paramExpected.DeepCopy()

	if expected, actual := len(bundleExpected.Spec.Resources), len(bundleActual.Spec.Resources); expected != actual {
		t.Logf("Found %d resources but expected %d resources", actual, expected)
		data, err := yaml.Marshal(bundleActual)
		require.NoError(t, err)
		t.Fatalf("Unexpected Bundle contents:\n%s", data)
	}

	for i, resActual := range bundleActual.Spec.Resources {
		actual := &resActual
		expected := &bundleExpected.Spec.Resources[i]

		ObjectCompareContext(t, fileName, actual.Spec.Object, expected.Spec.Object)
		PluginCompareContext(t, fileName, actual.Spec.Plugin, expected.Spec.Plugin)

		ResourceCompareContext(t, fileName, actual, expected)
	}
	bundleActual.Spec.Resources = nil
	bundleExpected.Spec.Resources = nil

	YAMLCompareContext(t, fileName, bundleExpected, bundleActual)
}
