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
	_ wiringplugin.Shape = &BindableEnvironmentVariables{}
	_ wiringplugin.Shape = &BindableIamAccessible{}
	_ wiringplugin.Shape = &IngressEndpoint{}
	_ wiringplugin.Shape = &SetOfPodsSelectableByLabels{}
	_ wiringplugin.Shape = &SnsSubscribable{}
)

func TestAllKnownShapes(t *testing.T) {
	t.Parallel()

	allKnownShapes := []wiringplugin.Shape{
		NewBindableEnvironmentVariables(resourceName, "abc", map[string]string{"a": "b"}),
		NewBindableIamAccessible(resourceName, "somePath"),
		NewIngressEndpoint(resourceName),
		NewSetOfPodsSelectableByLabels(resourceName, map[string]string{"a": "b"}),
		NewSnsSubscribable(resourceName),
	}

	for _, shape := range allKnownShapes {
		t.Run(string(shape.Name()), func(t *testing.T) {
			plugin_testing.TestShape(t, shape)
		})
	}
}
