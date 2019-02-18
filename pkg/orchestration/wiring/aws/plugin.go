// Package aws implements autowiring for the osb-aws-provider (Atlassian specific)
//
// This provider is just the old resource-provisioning service with an OSB interface slapped on.
// It therefore provides a number of different services/plans, but all work in the same way,
// accepting a format based on the original format RPS requires, which includes a 'ServiceEnvironment'
// field for location level variables and service specific globals (e.g. vpc, subnets, pagerduty...).
// However, the schemas of individual resources only expect the variables that are actually required
// for that resource (see oap.ResourceTypes).
package aws

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

type ServiceEnvironmentGenerator func(env *oap.ServiceEnvironment) *oap.ServiceEnvironment

// ShapesFunc is used to return a list of shapes for the resource to be used as
// input to the wiring functions of the dependants.
//
// The `resource` is the orchestration level resource that was transformed into `smithResource`.
type ShapesFunc func(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* external */, bool /* retriable */, error)

type WiringPlugin struct {
	clusterServiceClassExternalID servicecatalog.ClassExternalID
	clusterServicePlanExternalID  servicecatalog.PlanExternalID
	resourceType                  voyager.ResourceType
	shapes                        ShapesFunc

	OAPResourceTypeName        oap.ResourceType
	generateServiceEnvironment ServiceEnvironmentGenerator

	VPC func(location voyager.Location) *oap.VPCEnvironment
}

func Resource(resourceType voyager.ResourceType,
	oapResourceTypeName oap.ResourceType,
	clusterServiceClassExternalID servicecatalog.ClassExternalID,
	clusterServicePlanExternalID servicecatalog.PlanExternalID,
	generateServiceEnvironment ServiceEnvironmentGenerator,
	shapes ShapesFunc,
	vpc func(voyager.Location) *oap.VPCEnvironment,
) *WiringPlugin {
	wiringPlugin := &WiringPlugin{
		clusterServiceClassExternalID: clusterServiceClassExternalID,
		clusterServicePlanExternalID:  clusterServicePlanExternalID,
		resourceType:                  resourceType,
		shapes:                        shapes,
		OAPResourceTypeName:           oapResourceTypeName,
		generateServiceEnvironment:    generateServiceEnvironment,
		VPC:                           vpc,
	}
	return wiringPlugin
}

func (p *WiringPlugin) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != p.resourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, p.clusterServiceClassExternalID, p.clusterServicePlanExternalID)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	instanceParameters, external, retriable, err := p.instanceParameters(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}
	if instanceParameters != nil {
		serviceInstance.Spec.Parameters = &runtime.RawExtension{
			Raw: instanceParameters,
		}
	}

	instanceResourceName := wiringutil.ServiceInstanceResourceName(resource.Name)

	smithResource := smith_v1.Resource{
		Name:       instanceResourceName,
		References: nil, // no references
		Spec: smith_v1.ResourceSpec{
			Object: serviceInstance,
		},
	}

	shapes, external, retriable, err := p.shapes(resource, &smithResource, context)
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
		Resources: []smith_v1.Resource{smithResource},
	}
}

func (p *WiringPlugin) instanceParameters(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* externalError */, bool /* retriableError */, error) {
	rawAttributes, external, retriable, err := oap.BuildAttributes(resource.Spec, resource.Defaults)
	if err != nil {
		return nil, external, retriable, err
	}

	var attributes []byte
	if len(rawAttributes) != 0 {
		// only serialize attributes an non empty object
		attributes, err = json.Marshal(rawAttributes)
		if err != nil {
			return nil, false, false, errors.WithStack(err)
		}
	}

	alarms, err := oap.Alarms(resource.Spec)
	if err != nil {
		return nil, false, false, err
	}

	userServiceName, err := oap.ServiceName(resource.Spec)
	if err != nil {
		return nil, false, false, err
	}

	resourceName, err := oap.ResourceName(resource.Spec)
	if err != nil {
		return nil, false, false, err
	}
	if resourceName == "" {
		resourceName = string(resource.Name)
	}

	serviceName := serviceName(userServiceName, context)
	vpc := p.VPC(context.StateContext.Location)
	environment := p.generateServiceEnvironment(oap.MakeServiceEnvironmentFromContext(context, vpc))
	return instanceSpec(serviceName, resourceName, p.OAPResourceTypeName, *environment, attributes, alarms)
}

func serviceName(userServiceName voyager.ServiceName, context *wiringplugin.WiringContext) voyager.ServiceName {
	var serviceName voyager.ServiceName
	if userServiceName != "" {
		serviceName = userServiceName
	} else {
		serviceName = context.StateContext.ServiceName
	}
	return serviceName
}

func instanceSpec(serviceName voyager.ServiceName, resourceName string, oapName oap.ResourceType, environment oap.ServiceEnvironment, attributes, alarms []byte) ([]byte, bool, bool, error) {
	serviceInstanceSpec := oap.ServiceInstanceSpec{
		ServiceName: serviceName,
		Resource: oap.RPSResource{
			Name:       resourceName,
			Type:       string(oapName),
			Attributes: attributes,
			Alarms:     alarms,
		},
		Environment: environment,
	}
	serviceInstanceSpecBytes, err := json.Marshal(&serviceInstanceSpec)
	if err != nil {
		return nil, false, false, err
	}

	return serviceInstanceSpecBytes, false, false, nil
}
