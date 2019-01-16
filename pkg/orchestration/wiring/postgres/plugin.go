package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/rds"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "Postgres"

	clusterServiceClassExternalID = "8e14a988-0532-49ed-a6cd-31fa0c0fb2a8"
	clusterServicePlanExternalID  = "10aa2cb5-897d-43f6-b0df-ac4f8a2a758e"
	deletionDelay                 = 7 * 24 * time.Hour

	postgresEnvResourcePrefix = "pg"
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
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  instanceSpec,
			ObjectMeta:                    objectMeta,
			References:                    references,
			ResourceType:                  ResourceType,
			OptionalShapes:                svccatentangler.NoOptionalShapes,
		},
	}
}

func getRDSDependency(dependencies []wiringplugin.WiredDependency) (wiringplugin.WiredDependency, error) {
	rdsDependency := []wiringplugin.WiredDependency{}
	if len(dependencies) > 0 {
		for _, d := range dependencies {
			if d.Type == rds.ResourceType {
				rdsDependency = append(rdsDependency, d)
			}
		}
		if len(rdsDependency) > 1 {
			return wiringplugin.WiredDependency{}, errors.Errorf("Postgres resources can only depend on one RDS resource type")
		}
		if len(rdsDependency) == 1 {
			return rdsDependency[0], nil
		}
	}
	return wiringplugin.WiredDependency{}, nil
}

func references(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]smith_v1.Reference, error) {
	references := []smith_v1.Reference{}
	rdsDependency, err := getRDSDependency(context.Dependencies)
	if err != nil {
		return references, err
	}
	if rdsDependency.Name == "" {
		return references, nil
	}
	instanceName := wiringutil.ServiceInstanceResourceName(rdsDependency.Name)
	referenceName := wiringutil.ReferenceName(instanceName, "metadata-name")
	references = append(references, smith_v1.Reference{
		Name:     referenceName,
		Resource: wiringutil.ServiceInstanceResourceName(rdsDependency.Name),
		Path:     "metadata.name",
		Example:  "myownrds",
	})
	return references, nil
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var ourSpec autowiredOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &ourSpec); err != nil {
			return nil, errors.WithStack(err)
		}
		if ourSpec != (autowiredOnlySpec{}) {
			return nil, errors.Errorf("at least one autowired value not empty: %+v", ourSpec)
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
		return nil, errors.WithStack(err)
	}

	// Add to the user fields - let the user fields win!
	if resource.Spec != nil {
		var userSpec map[string]interface{}
		if err = json.Unmarshal(resource.Spec.Raw, &userSpec); err != nil {
			return nil, errors.WithStack(err)
		}

		// Emperor only understands 'lessee' instead of 'serviceName', so need a bit of translation here
		if userServiceName := userSpec["serviceName"]; userServiceName != nil {
			userSpec["lessee"] = userServiceName
			delete(userSpec, "serviceName")
		}

		if finalSpec, err = wiringutil.Merge(userSpec, finalSpec); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Filter instanceId
	delete(finalSpec, "instanceId")

	if finalSpec == nil {
		return nil, nil
	}

	// if this postgres depends on Dedicated RDS, it means it should be created there instead on the Default RDS
	// there should be only one RDS dependency
	rdsDependency, err := getRDSDependency(context.Dependencies)
	if err != nil {
		return nil, err
	}
	if rdsDependency.Name != "" {
		referenceName := wiringutil.ReferenceName(
			wiringutil.ServiceInstanceResourceName(rdsDependency.Name),
			"metadata-name",
		)
		shareddb := &SharedDbSpec{
			ResourceName: fmt.Sprintf("!{%s}", referenceName),
			ServiceName:  context.StateContext.ServiceName,
		}
		finalSpec["shareddb"] = shareddb
	}

	return json.Marshal(finalSpec)
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error) {
	return meta_v1.ObjectMeta{
		Annotations: map[string]string{
			voyager.Domain + "/envResourcePrefix": postgresEnvResourcePrefix,
			smith.DeletionDelayAnnotation:         deletionDelay.String(),
		},
	}, nil
}
