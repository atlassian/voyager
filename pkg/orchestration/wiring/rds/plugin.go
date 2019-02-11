package rds

import (
	"encoding/json"
	"reflect"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceType voyager.ResourceType = "RDS"

	clusterServiceClassExternalID = "d508783c-eef6-46fe-8245-d595ef2795e2"
	clusterServicePlanExternalID  = "7e6d37bb-8aa4-4c63-87d2-d78ca91a0120"
)

// MICROS Provided RDS CFN Parameters
type MainParametersSpec struct {
	MicrosAlarmEndpoints        []oap.MicrosAlarmSpec `json:"MicrosAlarmEndpoints"`
	MicrosAppSubnets            []string              `json:"MicrosAppSubnets"`
	MicrosEnv                   string                `json:"MicrosEnv"`
	MicrosEnvironmentLabel      string                `json:"MicrosEnvironmentLabel,omitempty"`
	MicrosInstanceSecurityGroup string                `json:"MicrosInstanceSecurityGroup"`
	MicrosJumpboxSecurityGroup  string                `json:"MicrosJumpboxSecurityGroup"`
	MicrosPagerdutyEndpoint     string                `json:"MicrosPagerdutyEndpoint,omitempty"`
	MicrosPagerdutyEndpointHigh string                `json:"MicrosPagerdutyEndpointHigh,omitempty"`
	MicrosPagerdutyEndpointLow  string                `json:"MicrosPagerdutyEndpointLow,omitempty"`
	MicrosPrivateDNSZone        string                `json:"MicrosPrivateDnsZone"`
	MicrosPrivatePaaSDNSZone    string                `json:"MicrosPrivatePaasDnsZone"`
	MicrosResourceName          string                `json:"MicrosResourceName"`
	MicrosServiceName           voyager.ServiceName   `json:"MicrosServiceName"`
	MicrosVPCId                 string                `json:"MicrosVpcId"`
}

type MiscParametersSpec struct {
	RDSType      string                 `json:"rds_type"`
	Tags         map[voyager.Tag]string `json:"tags"`
	Lessee       string                 `json:"lessee"`
	ResourceName string                 `json:"resource_name"`
}

type LocationSpec struct {
	Environment string `json:"env"`
}

type FinalSpec struct {
	PrimaryParameters MainParametersSpec `json:"primary_parameters"`
	Parameters        json.RawMessage    `json:"parameters"`
	Misc              MiscParametersSpec `json:"misc"`
	Location          LocationSpec       `json:"location"`
}

type AutowiredOnlySpec struct {
	PrimaryParameters MainParametersSpec `json:"primary_parameters"`
	Misc              MiscParametersSpec `json:"misc"`
	Location          LocationSpec       `json:"location"`
}

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

type ReadReplicaParam struct {
	ReadReplica bool `json:"ReadReplica"`
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  instanceSpec,
			ObjectMeta:                    objectMeta,
			ResourceType:                  ResourceType,
			Shapes:                        shapes,
		},
	}
}

func shapes(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* external */, bool /* retriable */, error) {
	si := smithResource.Spec.Object.(*sc_v1b1.ServiceInstance)
	var finalSpec FinalSpec
	err := json.Unmarshal(si.Spec.Parameters.Raw, &finalSpec)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	var readReplicaParam ReadReplicaParam
	err = json.Unmarshal(finalSpec.Parameters, &readReplicaParam)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	return []wiringplugin.Shape{
		knownshapes.NewSharedDbShape(smithResource.Name, readReplicaParam.ReadReplica),
	}, false, false, nil
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* external */, bool /* retriable */, error) {

	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var autoWiredOnly AutowiredOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &autoWiredOnly); err != nil {
			return nil, false, false, errors.WithStack(err)
		}
		if !reflect.DeepEqual(autoWiredOnly, AutowiredOnlySpec{}) {
			// this is a user error caused by an invalid spec
			return nil, true, false, errors.Errorf("at least one autowired value not empty: %+v", autoWiredOnly)
		}
	}

	// EMP-712: We are currently constructing the list of alarm endpoints manually.
	// When the alarmEndpoints list is available in context.StateContext, we should
	// just pass that down instead.
	microsAlarmEndpoints := oap.PagerdutyAlarmEndpoints(
		context.StateContext.ServiceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		context.StateContext.ServiceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch)

	primaryParameters := MainParametersSpec{
		MicrosAlarmEndpoints:        microsAlarmEndpoints,
		MicrosAppSubnets:            context.StateContext.LegacyConfig.AppSubnets,
		MicrosEnv:                   context.StateContext.LegacyConfig.MicrosEnv,
		MicrosEnvironmentLabel:      string(context.StateContext.Location.Label),
		MicrosInstanceSecurityGroup: context.StateContext.LegacyConfig.InstanceSecurityGroup,
		MicrosJumpboxSecurityGroup:  context.StateContext.LegacyConfig.JumpboxSecurityGroup,
		MicrosPagerdutyEndpoint:     context.StateContext.ServiceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		MicrosPagerdutyEndpointHigh: context.StateContext.ServiceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		MicrosPagerdutyEndpointLow:  context.StateContext.ServiceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch,
		MicrosPrivateDNSZone:        context.StateContext.LegacyConfig.Private,
		MicrosPrivatePaaSDNSZone:    context.StateContext.LegacyConfig.PrivatePaas,
		MicrosServiceName:           context.StateContext.ServiceName,
		MicrosResourceName:          string(resource.Name),
		MicrosVPCId:                 context.StateContext.LegacyConfig.Vpc,
	}

	miscParameters := MiscParametersSpec{
		RDSType:      "dedicated",
		Tags:         context.StateContext.Tags,
		Lessee:       string(context.StateContext.ServiceName),
		ResourceName: string(resource.Name),
	}

	location := LocationSpec{
		Environment: context.StateContext.LegacyConfig.MicrosEnv,
	}

	finalSpec := FinalSpec{
		PrimaryParameters: primaryParameters,
		Misc:              miscParameters,
		Location:          location,
	}

	// Emperor only understands 'lessee' instead of 'serviceName', so need a bit of translation here
	if resource.Spec != nil {
		// Try to unmarshal to do transformation later
		var userSpec map[string]interface{}
		if err := json.Unmarshal(resource.Spec.Raw, &userSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}

		if userServiceName := userSpec["serviceName"]; userServiceName != nil {
			userServiceNameStr, ok := userServiceName.(string)
			if !ok {
				// this is user error caused by an invalid spec
				return nil, true, false, errors.Errorf(`cannot unmarshal "serviceName" field: expected string got %T`, userServiceName)
			}
			delete(userSpec, "serviceName")
			finalSpec.Misc.Lessee = userServiceNameStr
		}

		// Marshall userSpec back to raw type
		raw, err := json.Marshal(userSpec)
		if err != nil {
			return nil, false, false, errors.WithStack(err)
		}
		finalSpec.Parameters = raw
	} else {
		finalSpec.Parameters = []byte("{}")
	}

	bytes, err := json.Marshal(finalSpec)
	return bytes, false, false, err
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, bool, bool, error) {
	return meta_v1.ObjectMeta{}, false, false, nil
}
