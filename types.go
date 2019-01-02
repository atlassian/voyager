package voyager

import (
	"fmt"
)

const (
	Domain      = "voyager.atl-paas.net"
	ScopeGlobal = "global"

	ServiceNameLabel  = Domain + "/serviceName"
	ServiceLabelLabel = Domain + "/label"
)

type Region string
type EnvType string
type Account string
type Label string

const (
	EnvTypeDev        EnvType = "dev"
	EnvTypeStaging    EnvType = "staging"
	EnvTypeProduction EnvType = "prod"
)

type ServiceName string

// +genset=true
type Location struct {
	EnvType EnvType `json:"envType"`
	Account Account `json:"account"`
	Region  Region  `json:"region"`
	// This is an 'environment/namespace' label, NOT a kubernetes label.
	Label Label `json:"label,omitempty"`
}

func (l Location) ClusterLocation() ClusterLocation {
	return ClusterLocation{
		Account: l.Account,
		Region:  l.Region,
		EnvType: l.EnvType,
	}
}

func (l Location) String() string {
	// This echoes the domain name form in Micros.
	s := fmt.Sprintf("%s.%s (account: %s)", l.Region, l.EnvType, l.Account)
	if l.Label != "" {
		s = string(l.Label) + "--" + s
	}
	return s
}

// This is basically an AWS tag.
type Tag string

func TagMapToStringMap(tagMap map[Tag]string) map[string]string {
	result := make(map[string]string, len(tagMap))
	for k, v := range tagMap {
		result[string(k)] = v
	}
	return result
}

type ResourceName string
type ResourceType string

// +genset=true
type ClusterLocation struct {
	EnvType EnvType
	Account Account
	Region  Region
}

func (cl ClusterLocation) String() string {
	// This echoes the domain name form in Micros.
	return fmt.Sprintf("%s.%s (account: %s)", cl.Region, cl.EnvType, cl.Account)
}

func (cl ClusterLocation) Location(label Label) Location {
	return Location{
		EnvType: cl.EnvType,
		Region:  cl.Region,
		Account: cl.Account,
		Label:   label,
	}
}
