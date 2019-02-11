package knownshapes

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	plugin_testing "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin/testing"
)

const (
	resourceName smith_v1.ResourceName = "res1"
)

var (
	_ wiringplugin.Shape = &ASAPKey{}
	_ wiringplugin.Shape = &BindableEnvironmentVariables{}
	_ wiringplugin.Shape = &BindableIamAccessible{}
	_ wiringplugin.Shape = &IngressEndpoint{}
	_ wiringplugin.Shape = &SetOfPodsSelectableByLabels{}
	_ wiringplugin.Shape = &SharedDb{}
	_ wiringplugin.Shape = &SnsSubscribable{}
)

func TestAllKnownShapes(t *testing.T) {
	t.Parallel()

	allKnownShapes := []wiringplugin.Shape{
		NewASAPKey(),
		NewBindableEnvironmentVariables(resourceName, "abc", map[string]string{"a": "b"}),
		NewBindableIamAccessible(resourceName, "somePath"),
		NewIngressEndpoint(resourceName),
		NewSetOfPodsSelectableByLabels(resourceName, map[string]string{"a": "b"}),
		NewSetOfDatadog(resourceName),
		NewSharedDbShape(resourceName, true),
		NewSnsSubscribable(resourceName),

		&wiringplugin.UnstructuredShape{
			ShapeMeta: wiringplugin.ShapeMeta{
				ShapeName: "somename",
			},
			Data: map[string]interface{}{
				"a": "b",
				"b": int64(5),
				"c": map[string]interface{}{
					"x": "z",
				},
				"d": []interface{}{
					"1",
				},
				"e": float64(1.1),
			},
		},
	}

	for _, shape := range allKnownShapes {
		t.Run(string(shape.Name()), func(t *testing.T) {
			plugin_testing.TestShape(t, shape)
		})
	}
}
