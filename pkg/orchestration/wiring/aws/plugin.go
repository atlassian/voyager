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

	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceEnvironmentGenerator func(env *oap.ServiceEnvironment) *oap.ServiceEnvironment

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler

	OAPResourceTypeName        oap.ResourceType
	generateServiceEnvironment ServiceEnvironmentGenerator
	envResourcePrefix          oap.EnvVarPrefix
}

func Resource(resourceType voyager.ResourceType,
	oapResourceTypeName oap.ResourceType,
	clusterServiceClassExternalID servicecatalog.ClassExternalID,
	clusterServicePlanExternalID servicecatalog.PlanExternalID,
	generateServiceEnvironment ServiceEnvironmentGenerator,
	envResourcePrefix oap.EnvVarPrefix,
) *WiringPlugin {
	wiringPlugin := &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			ResourceType:                  resourceType,
		},
		OAPResourceTypeName:        oapResourceTypeName,
		generateServiceEnvironment: generateServiceEnvironment,
		envResourcePrefix:          envResourcePrefix,
	}
	wiringPlugin.SvcCatEntangler.InstanceSpec = wiringPlugin.instanceSpec
	wiringPlugin.SvcCatEntangler.ObjectMeta = wiringPlugin.objectMeta
	return wiringPlugin
}

func (awp *WiringPlugin) instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	rawAttributes, err := oap.BuildAttributes(resource.Spec, resource.Defaults)
	if err != nil {
		return nil, err
	}

	var attributes []byte
	if len(rawAttributes) != 0 {
		// only serialize attributes an non empty object
		attributes, err = json.Marshal(rawAttributes)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	alarms, err := oap.Alarms(resource.Spec)
	if err != nil {
		return nil, err
	}

	userServiceName, err := oap.ServiceName(resource.Spec)
	if err != nil {
		return nil, err
	}

	resourceName, err := oap.ResourceName(resource.Spec)
	if err != nil {
		return nil, err
	}
	if resourceName == "" {
		resourceName = string(resource.Name)
	}

	serviceName := serviceName(userServiceName, context)
	environment := awp.generateServiceEnvironment(oap.MakeServiceEnvironmentFromContext(context))
	return instanceSpec(serviceName, resourceName, awp.OAPResourceTypeName, *environment, attributes, alarms)
}

func (awp *WiringPlugin) objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error) {
	return objectMeta(awp.envResourcePrefix), nil
}

func serviceName(userServiceName string, context *wiringplugin.WiringContext) string {
	serviceName := context.StateContext.ServiceName
	if userServiceName != "" {
		serviceName = userServiceName
	}
	return serviceName
}

func instanceSpec(serviceName string, resourceName string, oapName oap.ResourceType, environment oap.ServiceEnvironment, attributes, alarms []byte) ([]byte, error) {
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
		return nil, err
	}

	return serviceInstanceSpecBytes, nil
}

func objectMeta(prefix oap.EnvVarPrefix) meta_v1.ObjectMeta {
	annotations := make(map[string]string)
	if prefix != "" {
		// This is needed for compatibility with former Micros RPS resource types
		// that generate environment variables with custom prefix
		// e.g. DYNAMO_* instead of DYNAMO_DB_* for dynamo-db
		annotations[voyager.Domain+"/envResourcePrefix"] = string(prefix)
	}
	return meta_v1.ObjectMeta{Annotations: annotations}
}
