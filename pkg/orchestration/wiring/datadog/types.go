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

type AlarmAttributes struct {
	Name    string        `json:"name"`
	Type    AlarmSpecType `json:"type"`
	Query   string        `json:"query"`
	Options AlarmOptions  `json:"options"`
	Message string        `json:"message"`
}

type OSBInstanceParameters struct {
	ServiceName voyager.ServiceName `json:"serviceName"`
	EnvType     voyager.EnvType     `json:"envType"`
	Region      voyager.Region      `json:"region"`
	Label       voyager.Label       `json:"label,omitempty"`
	Attributes  AlarmAttributes     `json:"attributes"`
}
type AlarmOptions struct {
	Thresholds        *AlarmThresholds `json:"thresholds,omitempty"`
	NewHostDelay      *int32           `json:"new_host_delay,omitempty"`
	RenotifyInterval  *int32           `json:"renotify_interval,omitempty"`
	TimeoutSeconds    *int32           `json:"timeout_h,omitempty"`
	EscalationMessage string           `json:"escalation_message,omitempty"`
	NoDataTimeFrame   string           `json:"no_data_timeframe,omitempty"`
	Locked            bool             `json:"locked,omitempty"`
	NotifyNoData      bool             `json:"notify_no_data,omitempty"`
	NotifyAudit       bool             `json:"notify_audit,omitempty"`
	RequireFullWindow bool             `json:"require_full_window,omitempty"`
}

type AlarmThresholds struct {
	Critical int32 `json:"critical,omitempty"`
	Warning  int32 `json:"warning,omitempty"`
}

type QueryParams struct {
	KubeDeployment string           `json:"kubeDeployment,omitempty"`
	KubeNamespace  string           `json:"kubeNamespace,omitempty"`
	Threshold      *AlarmThresholds `json:"threshold,omitempty"`
	AlarmType      AlarmType        `json:"alarmType,omitempty"`
	Location       voyager.Location `json:"location,omitempty"`
}
