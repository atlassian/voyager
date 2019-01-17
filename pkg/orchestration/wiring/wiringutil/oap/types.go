package oap

import (
	"encoding/json"

	"github.com/atlassian/voyager"
)

type EnvVarPrefix string
type ResourceType string
type CfnTemplate string

type ServiceInstanceSpec struct {
	ServiceName voyager.ServiceName `json:"serviceName"`
	Resource    RPSResource         `json:"resource"`
	Environment ServiceEnvironment  `json:"environment"`
}

type ServiceEnvironment struct {
	NotificationEmail     string                 `json:"notificationEmail,omitempty"`
	AlarmEndpoints        []MicrosAlarmSpec      `json:"alarmEndpoints,omitempty"`
	Tags                  map[voyager.Tag]string `json:"tags,omitempty"`
	ServiceSecurityGroup  string                 `json:"serviceSecurityGroup,omitempty"`
	PrimaryVpcEnvironment *VPCEnvironment        `json:"primaryVpcEnvironment,omitempty"`
	Fallback              *bool                  `json:"fallback,omitempty"`
}

type VPCEnvironment struct {
	VPCID                 string   `json:"vpcId,omitempty"`
	PrivateDNSZone        string   `json:"privateDnsZone,omitempty"`
	PrivatePaasDNSZone    string   `json:"privatePaasDnsZone,omitempty"`
	ServiceSecurityGroup  string   `json:"serviceSecurityGroup,omitempty"`
	InstanceSecurityGroup string   `json:"instanceSecurityGroup,omitempty"`
	JumpboxSecurityGroup  string   `json:"jumpboxSecurityGroup,omitempty"`
	SSLCertificateID      string   `json:"sslCertificateId,omitempty"`
	Label                 string   `json:"label,omitempty"`
	AppSubnets            []string `json:"appSubnets,omitempty"`
	Zones                 []string `json:"zones,omitempty"`
	Region                string   `json:"region,omitempty"`
	EMRSubnet             string   `json:"emrSubnet,omitempty"`
}

type RPSResource struct {
	Type string `json:"type"`
	Name string `json:"name"`

	Attributes json.RawMessage `json:"attributes,omitempty"`
	Alarms     json.RawMessage `json:"alarms,omitempty"`
}

type MicrosAlarmSpec struct {
	Type     string `json:"type"`
	Priority string `json:"priority"`
	Endpoint string `json:"endpoint"`
	Consumer string `json:"consumer"`
}

