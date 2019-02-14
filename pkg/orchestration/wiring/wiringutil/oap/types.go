package oap

import (
	"encoding/json"

	"github.com/atlassian/voyager"
)

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
	VPCID                 string         `json:"vpcId,omitempty"`
	PrivateDNSZone        string         `json:"privateDnsZone,omitempty"`
	PrivatePaasDNSZone    string         `json:"privatePaasDnsZone,omitempty"`
	InstanceSecurityGroup string         `json:"instanceSecurityGroup,omitempty"`
	JumpboxSecurityGroup  string         `json:"jumpboxSecurityGroup,omitempty"`
	SSLCertificateID      string         `json:"sslCertificateId,omitempty"`
	Label                 voyager.Label  `json:"label,omitempty"`
	AppSubnets            []string       `json:"appSubnets,omitempty"`
	Zones                 []string       `json:"zones,omitempty"`
	Region                voyager.Region `json:"region,omitempty"`
	EMRSubnet             string         `json:"emrSubnet,omitempty"`
}

var ExampleVPC = func(label voyager.Label, region voyager.Region) *VPCEnvironment {
	return &VPCEnvironment{
		VPCID:                 "vpc-1",
		PrivateDNSZone:        "testregion.atl-inf.io",
		PrivatePaasDNSZone:    "testregion.dev.paas-inf.net",
		InstanceSecurityGroup: "sg-2",
		JumpboxSecurityGroup:  "sg-1",
		SSLCertificateID:      "arn:aws:acm:testregion:123456789012:certificate/253b42fa-047c-44c2-8bac-777777777777",
		Label:                 label,
		AppSubnets:            []string{"subnet-1", "subnet-2"},
		Zones:                 []string{"testregiona", "testregionb"},
		Region:                region,
		EMRSubnet:             "subnet-1a",
	}
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
