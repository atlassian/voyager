package oap

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

func MakeServiceEnvironmentFromContext(context *wiringplugin.WiringContext) *ServiceEnvironment {
	config := context.StateContext.LegacyConfig
	location := context.StateContext.Location
	serviceProperties := context.StateContext.ServiceProperties
	alarmEndpoints := PagerdutyAlarmEndpoints(
		serviceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		serviceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch)

	return &ServiceEnvironment{
		NotificationEmail: serviceProperties.Notifications.Email,
		AlarmEndpoints:    alarmEndpoints,
		Tags:              context.StateContext.Tags,
		PrimaryVpcEnvironment: &VPCEnvironment{
			VPCID:                 config.Vpc,
			JumpboxSecurityGroup:  config.JumpboxSecurityGroup,
			InstanceSecurityGroup: config.InstanceSecurityGroup,
			SSLCertificateID:      config.CertificateID,
			PrivateDNSZone:        config.Private,
			PrivatePaasDNSZone:    config.PrivatePaas,
			Label:                 location.Label,
			AppSubnets:            config.AppSubnets,
			Zones:                 config.Zones,
			Region:                location.Region,
			EMRSubnet:             config.EMRSubnet,
		},
	}
}

func PagerdutyAlarmEndpoints(highPriorityPagerdutyEndpoint string, lowPriorityPagerdutyEndpoint string) []MicrosAlarmSpec {
	return []MicrosAlarmSpec{
		{
			Type:     "CloudWatch",
			Priority: "high",
			Endpoint: highPriorityPagerdutyEndpoint,
			Consumer: "pagerduty",
		},
		{
			Type:     "CloudWatch",
			Priority: "low",
			Endpoint: lowPriorityPagerdutyEndpoint,
			Consumer: "pagerduty",
		},
	}
}
