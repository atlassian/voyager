package oap

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

func MakeServiceEnvironmentFromContext(context *wiringplugin.WiringContext) *ServiceEnvironment {
	config := context.StateContext.LegacyConfig
	location := context.StateContext.Location
	serviceProperties := context.StateContext.ServiceProperties

	return &ServiceEnvironment{
		NotificationEmail:            serviceProperties.Notifications.Email,
		LowPriorityPagerdutyEndpoint: serviceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch,
		PagerdutyEndpoint:            serviceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		Tags:                         context.StateContext.Tags,
		PrimaryVpcEnvironment: &VPCEnvironment{
			VPCID:                 config.Vpc,
			JumpboxSecurityGroup:  config.JumpboxSecurityGroup,
			InstanceSecurityGroup: config.InstanceSecurityGroup,
			SSLCertificateID:      config.CertificateID,
			PrivateDNSZone:        config.Private,
			PrivatePaasDNSZone:    config.PrivatePaas,
			Label:                 string(location.Label),
			AppSubnets:            config.AppSubnets,
			Zones:                 config.Zones,
			Region:                string(location.Region),
			EMRSubnet:             config.EMRSubnet,
		},
	}
}
