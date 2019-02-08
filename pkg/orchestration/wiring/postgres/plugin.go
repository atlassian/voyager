package postgres

import (
	"encoding/json"
	"fmt"
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
	ServiceName  voyager.ServiceName `json:"service"`
	ResourceName string              `json:"resource"`
}

type autowiredOnlySpec struct {
	ResourceName string       `json:"resource_name"`
	Location     LocationSpec `json:"location"`
	SharedDb     SharedDbSpec `json:"shareddb"`
}

type partialSpec struct {
	ResourceName string       `json:"resource_name"`
	Lessee       string       `json:"lessee"`
	Location     LocationSpec `json:"location"`

	// Note that users can add extra parameters that are not validated here
	// Currently emperor only supports target_rds_instance
}

type LocationSpec struct {
	Environment string `json:"env"`
}

type WiringPlugin struct {
}

func New() *WiringPlugin {
	return &WiringPlugin{}
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

	instanceParameters, external, retriable, err := instanceParameters(resource, context)
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

	references, external, retriable, err := references(context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	serviceInstanceResource := smith_v1.Resource{
		Name:       wiringutil.ServiceInstanceResourceName(resource.Name),
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: serviceInstance,
		},
	}

	shapes, external, retriable, err := shapes(resource, &serviceInstanceResource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: shapes,
		},
		Resources: []smith_v1.Resource{serviceInstanceResource},
	}
}

func shapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* external */, bool /* retriable */, error) {
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

	// If the postgres has an RDS dependency with read replica enabled, it
	// produces some additional environment variables for the read replica
	var foundSharedDb []*knownshapes.SharedDb
	for _, dep := range context.Dependencies {
		sharedDb, found, err := knownshapes.FindSharedDbShape(dep.Contract.Shapes)
		if err != nil {
			// this error is because the wiring return an invalid shape or had duplicate shapes - this is an internal error (code issue)
			return nil, false, false, err
		}
		if found {
			foundSharedDb = append(foundSharedDb, sharedDb)
		}
	}
	if len(foundSharedDb) > 1 {
		return nil, true, false, errors.Errorf("found more than one postgres dependency for %q", resource.Name)
	}
	if len(foundSharedDb) == 1 && foundSharedDb[0].Data.HasSameRegionReadReplica {
		envVars["READONLY_REPLICA"] = "data.readonly_replica"
		envVars["READONLY_REPLICA_URL"] = "data.readonly_replica_url"
	}

	return []wiringplugin.Shape{
		knownshapes.NewBindableEnvironmentVariables(smithResource.Name, postgresEnvResourcePrefix, envVars),
	}, false, false, nil
}

func references(context *wiringplugin.WiringContext) ([]smith_v1.Reference, bool /* external */, bool /* retriable */, error) {
	dep, found, err := context.FindTheOnlyDependency()
	if err != nil {
		return nil, false, false, err
	}
	// No dependencies
	if !found {
		return nil, true, false, nil
	}

	// Check if dependency has a RDS shape
	_, found, err = knownshapes.FindSharedDbShape(dep.Contract.Shapes)
	if err != nil {
		return nil, false, false, err
	}

	// Found dependency but it was not a RDS resource
	if !found {
		return nil, true, false, nil
	}

	instanceName := wiringutil.ServiceInstanceResourceName(dep.Name)
	referenceName := wiringutil.ReferenceName(instanceName, "metadata-name")

	return []smith_v1.Reference{{
		Name:     referenceName,
		Resource: wiringutil.ServiceInstanceResourceName(dep.Name),
		Path:     "metadata.name",
		Example:  "myownrds",
	}}, false, false, nil
}

func instanceParameters(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* externalErr */, bool /* retriable */, error) {
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
		Lessee:       string(context.StateContext.ServiceName),
		ResourceName: string(resource.Name),
		Location: LocationSpec{
			Environment: context.StateContext.LegacyConfig.MicrosEnv,
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

	if finalSpec == nil {
		return nil, false, false, nil
	}

	// if this postgres depends on Dedicated RDS, it means it should be created there instead on the Default RDS
	// there should be only one RDS dependency
	dep, found, err := context.FindTheOnlyDependency()
	if err != nil {
		return nil, false, false, err
	}

	// Did not find any dependencies
	if !found {
		bytes, err := json.Marshal(finalSpec)
		return bytes, false, false, err
	}

	_, found, err = knownshapes.FindSharedDbShape(dep.Contract.Shapes)
	if err != nil {
		return nil, false, false, errors.Wrapf(err, "unable to determine if shape %s db was a dependency", knownshapes.SharedDbShape)
	}

	if !found {
		// User error - dependency is wrong
		return nil, true, false, errors.Errorf("expected to find shape %s in %q", knownshapes.SharedDbShape, dep.Name)
	}

	referenceName := wiringutil.ReferenceName(
		wiringutil.ServiceInstanceResourceName(dep.Name),
		"metadata-name",
	)
	shareddb := &SharedDbSpec{
		ResourceName: fmt.Sprintf("!{%s}", referenceName),
		ServiceName:  context.StateContext.ServiceName,
	}
	finalSpec["shareddb"] = shareddb

	bytes, err := json.Marshal(finalSpec)
	return bytes, false, false, err
}
