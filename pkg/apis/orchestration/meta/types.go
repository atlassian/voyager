package meta

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/opsgenie"
)

const (
	ConfigMapConfigKey = "config"
)

// ServiceProperties describes additional metadata that we receive from a
// ConfigMap alongside the State resource.
// This metadata is usually data that can be preconfigured or managed separately
// from an actual deployment of a State.
// For example, you might use ServiceProperties to sync a directory of services
// to Kubernetes, such that any deployments will be able to pick up that data
// and pass it along to your autowiring functions.
// LEGACY: This is also hardcoded to Atlassian-looking things, but we could
//         make our autowiring plugins decode this?
type ServiceProperties struct {
	ResourceOwner   string                 `json:"resourceOwner"`
	BusinessUnit    string                 `json:"businessUnit"`
	Notifications   Notifications          `json:"notifications"`
	UserTags        map[voyager.Tag]string `json:"userTags,omitempty"`
	LoggingID       string                 `json:"loggingId"`
	SSAMAccessLevel string                 `json:"ssamAccessLevel"`
	Compliance      Compliance             `json:"compliance,omitempty"`
}

// Notification is used in the ServiceProperties.
type Notifications struct {
	Email                        string                 `json:"email"`
	LowPriorityPagerdutyEndpoint PagerDuty              `json:"lowPriority"`
	PagerdutyEndpoint            PagerDuty              `json:"main"`
	OpsgenieIntegrations         []opsgenie.Integration `json:"opsgenieIntegrations"`
}

// PagerDuty is used in the ServiceProperties.
type PagerDuty struct {
	Generic    string `json:"generic"`
	CloudWatch string `json:"cloudwatch"`
}

// Compliance includes all the service compliance related data
type Compliance struct {
	PRGBControl *bool `json:"prgbControl,omitempty"`
}
