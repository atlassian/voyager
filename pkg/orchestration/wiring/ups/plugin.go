package ups

import (
	"encoding/json"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
)

const (
	ResourceType   voyager.ResourceType = "UPS"
	ResourcePrefix                      = "UPS"

	clusterServiceClassExternalID = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468"
	clusterServicePlanExternalID  = "86064792-7ea2-467b-af93-ac9694d96d52"
)

var (
	envVarReplacer    = strings.NewReplacer("-", "_", ".", "_")
	defaultUpsEnvVars = map[string]string{
		"SPECIAL_KEY_1": "data.special-key-1",
		"SPECIAL_KEY_2": "data.special-key-2",
	}
)

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  InstanceSpec,
			ResourceType:                  ResourceType,
			Shapes:                        shapes,
		},
	}
}

func shapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, error) {
	// UPS outputs all of its inputs
	si := smithResource.Spec.Object.(*sc_v1b1.ServiceInstance)
	parameters := map[string]json.RawMessage{}
	if si.Spec.Parameters == nil {
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResource.Name, ResourcePrefix, defaultUpsEnvVars),
		}, nil
	}

	err := json.Unmarshal(si.Spec.Parameters.Raw, &parameters)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	credentials, ok := parameters["credentials"]
	if !ok {
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResource.Name, ResourcePrefix, defaultUpsEnvVars),
		}, nil
	}

	credentialsMap := map[string]string{}
	err = json.Unmarshal(credentials, &credentialsMap)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	envVars := map[string]string{}
	for k := range credentialsMap {
		envVars[makeEnvVarName(k)] = "data." + k
	}
	return []wiringplugin.Shape{
		knownshapes.NewBindableEnvironmentVariables(smithResource.Name, ResourcePrefix, envVars),
	}, nil
}

func makeEnvVarName(elements ...string) string {
	return strings.ToUpper(envVarReplacer.Replace(strings.Join(elements, "_")))
}

// Just a straight passthrough...
// (should probably just implement a default autowiring function similar to how RPS OSB works, which
// takes the class/plan names as arguments?)
func InstanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	if resource.Spec == nil {
		return nil, nil
	}

	return resource.Spec.Raw, nil
}
