package oap

import (
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

func MakeServiceEnvironmentFromContext(context *wiringplugin.WiringContext, vpc *VPCEnvironment) *ServiceEnvironment {
	serviceProperties := context.StateContext.ServiceProperties
	alarmEndpoints := PagerdutyAlarmEndpoints(
		serviceProperties.Notifications.PagerdutyEndpoint.CloudWatch,
		serviceProperties.Notifications.LowPriorityPagerdutyEndpoint.CloudWatch)

	return &ServiceEnvironment{
		NotificationEmail:     serviceProperties.Notifications.Email,
		AlarmEndpoints:        alarmEndpoints,
		Tags:                  context.StateContext.Tags,
		PrimaryVpcEnvironment: vpc,
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
