package rds

import (
	"encoding/json"
	"reflect"

	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ResourceType voyager.ResourceType = "RDS"

	clusterServiceClassExternalID = "d508783c-eef6-46fe-8245-d595ef2795e2"
	clusterServicePlanExternalID  = "7e6d37bb-8aa4-4c63-87d2-d78ca91a0120"

	rdsEnvResourcePrefix = "RDS"
)

// MICROS Provided RDS CFN Parameters
type MainParametersSpec struct {
	MicrosAppSubnets            []string            `json:"MicrosAppSubnets"`
	MicrosEnv                   string              `json:"MicrosEnv"`
	MicrosEnvironmentLabel      string              `json:"MicrosEnvironmentLabel,omitempty"`
	MicrosInstanceSecurityGroup string              `json:"MicrosInstanceSecurityGroup"`
	MicrosJumpboxSecurityGroup  string              `json:"MicrosJumpboxSecurityGroup"`
	MicrosPagerdutyEndpoint     string              `json:"MicrosPagerdutyEndpoint,omitempty"`
	MicrosPagerdutyEndpointHigh string              `json:"MicrosPagerdutyEndpointHigh,omitempty"`
	MicrosPagerdutyEndpointLow  string              `json:"MicrosPagerdutyEndpointLow,omitempty"`
	MicrosPrivateDNSZone        string              `json:"MicrosPrivateDnsZone"`
	MicrosPrivatePaaSDNSZone    string              `json:"MicrosPrivatePaasDnsZone"`
	MicrosResourceName          string              `json:"MicrosResourceName"`
	MicrosServiceName           voyager.ServiceName `json:"MicrosServiceName"`
	MicrosVPCId                 string              `json:"MicrosVpcId"`
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

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			ResourceType:                  ResourceType,
			InstanceSpec:                  instanceSpec,
			ObjectMeta:                    objectMeta,
		},
	}
}

func instanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {

	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var autoWiredOnly AutowiredOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &autoWiredOnly); err != nil {
			return nil, errors.WithStack(err)
		}
		if !reflect.DeepEqual(autoWiredOnly, AutowiredOnlySpec{}) {
			return nil, errors.Errorf("at least one autowired value not empty: %+v", autoWiredOnly)
		}
	}

	primaryParameters := MainParametersSpec{
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
			return nil, errors.WithStack(err)
		}

		if userServiceName := userSpec["serviceName"]; userServiceName != nil {
			userServiceNameStr, ok := userServiceName.(string)
			if !ok {
				return nil, errors.Errorf(`cannot unmarshal "serviceName" field: expected string got %T`, userServiceName)
			}
			delete(userSpec, "serviceName")
			finalSpec.Misc.Lessee = userServiceNameStr
		}

		// Marshall userSpec back to raw type
		raw, err := json.Marshal(userSpec)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		finalSpec.Parameters = raw
	} else {
		finalSpec.Parameters = []byte("{}")
	}

	return json.Marshal(finalSpec)
}

func objectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error) {
	return meta_v1.ObjectMeta{
		Annotations: map[string]string{
			voyager.Domain + "/envResourcePrefix": rdsEnvResourcePrefix,
		},
	}, nil
}
