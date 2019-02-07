package datadog

import (
	"github.com/atlassian/voyager"
)

type AlarmSpecType string
type AlarmType string

const (
	Metric AlarmSpecType = "metric alert"
)
const (
	CPU    AlarmType = "cpu"
	Memory AlarmType = "memory"
)

type Alarm struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Query   string      `json:"query"`
	Option  AlarmOption `json:"options"`
	Message string      `json:"message"`
}

type ServiceInstanceSpec struct {
	ServiceName voyager.ServiceName `json:"serviceName"`
	Attributes  Alarm               `json:"attributes"`
	Environment voyager.EnvType     `json:"environment"`
	Region      voyager.Region      `json:"region"`
}
type AlarmOption struct {
	Timeout           int             `json:"timeout_h,omitempty"`
	locked            bool            `json:"locked,omitempty"`
	NotifyNOData      bool            `json:"notify_no_data,omitempty"`
	NodDataTimeFrame  string          `json:"no_data_timeframe,omitempty"`
	NotifyAudit       bool            `json:"notify_audit,omitempty"`
	RequireFullWindow bool            `json:"require_full_window,omitempty"`
	NewHostDelay      int             `json:"new_host_delay,omitempty"`
	EscalationMessage string          `json:"escalation_message,omitempty"`
	RenotifyInterval  int             `json:"renotify_interval,omitempty"`
	Thresholds        AlarmThresholds `json:"thresholds,omitempty"`
}

type AlarmThresholds struct {
	Critical int32 `json:"critical"`
	Warning  int32 `json:"warning,omitempty"`
}

type QueryParams struct {
	KubeDeployment string
	KubeNamespace  string
	Env            string
	Region         string
	Threshold      *AlarmThresholds
	AlarmType      AlarmType
}
