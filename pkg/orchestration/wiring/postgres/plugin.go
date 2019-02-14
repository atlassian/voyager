package postgres

import (
	"encoding/json"
	"time"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "Postgres"

	clusterServiceClassExternalID = "8e14a988-0532-49ed-a6cd-31fa0c0fb2a8"
	clusterServicePlanExternalID  = "10aa2cb5-897d-43f6-b0df-ac4f8a2a758e"
	deletionDelay                 = 7 * 24 * time.Hour

	postgresEnvResourcePrefix = "PG"
)

// When the Postgres database should be created in a Dedicated RDS instance,
// Emperor expects this information in the provision payload, otherwise,
// if absent, it will be created in the Default RDS
type SharedDbSpec struct {
	ServiceName  voyager.ServiceName  `json:"service"`
	ResourceName voyager.ResourceName `json:"resource"`
}

type autowiredOnlySpec struct {
	ResourceName voyager.ResourceName `json:"resource_name"`
	Location     LocationSpec         `json:"location"`
	SharedDb     SharedDbSpec         `json:"shareddb"`
}

type partialSpec struct {
	ResourceName voyager.ResourceName `json:"resource_name"`
	Lessee       voyager.ServiceName  `json:"lessee"`
	Location     LocationSpec         `json:"location"`

	// Note that users can add extra parameters that are not validated here
	// Currently emperor only supports target_rds_instance
}

type LocationSpec struct {
	Environment string `json:"env"`
}

type WiringPlugin struct {
	Environment func(location voyager.Location) string
}

func New() *WiringPlugin {
	return &WiringPlugin{
		Environment: func(_ voyager.Location) string {
			return "microstestenv"
		},
	}
}

func (p *WiringPlugin) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, clusterServiceClassExternalID, clusterServicePlanExternalID)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	serviceInstance.ObjectMeta.Annotations = map[string]string{
		smith.DeletionDelayAnnotation: deletionDelay.String(),
	}

	// if this postgres depends on Dedicated RDS, it means it should be created there instead on the Default RDS
	// there should be only one RDS dependency
	sharedDbDep, found, err := context.FindTheOnlyDependency()
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}
	var references []smith_v1.Reference
	envVars := map[string]string{
		"HOST":         "data.host",
		"PORT":         "data.port",
		"SCHEMA":       "data.schema",
		"ROLE":         "data.role",
		"PASSWORD":     "data.password",
		"URL":          "data.url",
		"READROLE":     "data.readrole",
		"READPASSWORD": "data.readpassword",
		"READURL":      "data.readurl",
	}
	if found {
		var sharedDbShape *knownshapes.SharedDb
		sharedDbShape, found, err = knownshapes.FindSharedDbShape(sharedDbDep.Contract.Shapes)
		if err != nil {
			return &wiringplugin.WiringResultFailure{
				Error: errors.Wrapf(err, "unable to determine if shape %s db was a dependency", knownshapes.SharedDbShape),
			}
		}
		if !found {
			// User error - dependency is wrong
			return &wiringplugin.WiringResultFailure{
				Error:           errors.Errorf("expected to find shape %s in %q", knownshapes.SharedDbShape, sharedDbDep.Name),
				IsExternalError: true,
			}
		}
		if sharedDbShape.Data.HasSameRegionReadReplica {
			envVars["READONLY_REPLICA"] = "data.readonly_replica"
			envVars["READONLY_REPLICA_URL"] = "data.readonly_replica_url"
		}
		references = []smith_v1.Reference{
			{
				Resource: sharedDbShape.Data.ResourceName,
			},
		}
	}

	instanceParameters, external, retriable, err := p.instanceParameters(resource, context, sharedDbDep)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}
	serviceInstance.Spec.Parameters = &runtime.RawExtension{
		Raw: instanceParameters,
	}

	instanceResourceName := wiringutil.ServiceInstanceResourceName(resource.Name)

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: []wiringplugin.Shape{
				knownshapes.NewBindableEnvironmentVariables(instanceResourceName, postgresEnvResourcePrefix, envVars),
			},
		},
		Resources: []smith_v1.Resource{
			{
				Name:       instanceResourceName,
				References: references,
				Spec: smith_v1.ResourceSpec{
					Object: serviceInstance,
				},
			},
		},
	}
}

// instanceParameters constructs ServiceInstance parameters.
// sharedDbDep may be nil
func (p *WiringPlugin) instanceParameters(resource *orch_v1.StateResource, context *wiringplugin.WiringContext, sharedDbDep *wiringplugin.WiredDependency) ([]byte, bool /* externalErr */, bool /* retriable */, error) {
	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var ourSpec autowiredOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &ourSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}
		if ourSpec != (autowiredOnlySpec{}) {
			return nil, true, false, errors.Errorf("at least one autowired value not empty: %+v", ourSpec)
		}
	}

	// Build final spec, by combining calculated variables + user provided variables
	var finalSpec map[string]interface{}

	// Insert calculated variables
	finalSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&partialSpec{
		Lessee:       context.StateContext.ServiceName,
		ResourceName: resource.Name,
		Location: LocationSpec{
			Environment: p.Environment(context.StateContext.Location),
		},
	})
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	// Add to the user fields - let the user fields win!
	if resource.Spec != nil {
		var userSpec map[string]interface{}
		if err = json.Unmarshal(resource.Spec.Raw, &userSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}

		// Emperor only understands 'lessee' instead of 'serviceName', so need a bit of translation here
		if userServiceName := userSpec["serviceName"]; userServiceName != nil {
			userSpec["lessee"] = userServiceName
			delete(userSpec, "serviceName")
		}

		if finalSpec, err = wiringutil.Merge(userSpec, finalSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}
	}

	// Filter instanceId
	delete(finalSpec, "instanceId")

	if sharedDbDep == nil {
		// Does not depend on RDS
		bytes, err := json.Marshal(finalSpec)
		return bytes, false, false, err
	}

	finalSpec["shareddb"] = &SharedDbSpec{
		ResourceName: sharedDbDep.Name,
		ServiceName:  context.StateContext.ServiceName,
	}

	bytes, err := json.Marshal(finalSpec)
	return bytes, false, false, err
}
