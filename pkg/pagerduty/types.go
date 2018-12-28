package pagerduty

import (
	"github.com/atlassian/voyager/pkg/util"
)

type ClientConfig struct {
	AuthToken string
}

type IntegrationName string
type ServiceName string
type EscalationPolicy string

const (
	Generic    IntegrationName = "generic"
	CloudWatch IntegrationName = "cloudwatch"
	// Deprecated: the "pingdom" integration is deprecated, but kept for backward compatibility with Micros
	Pingdom IntegrationName = "pingdom"
)

func NewPagerDutyClientConfigFromEnv() (ClientConfig, error) {
	const (
		pagerdutyTokenKey = "PAGERDUTY_TOKEN"
	)

	vars, err := util.EnvironmentVariablesAsMap(pagerdutyTokenKey)
	if err != nil {
		return ClientConfig{}, err
	}
	return ClientConfig{
		AuthToken: vars[pagerdutyTokenKey],
	}, nil
}
