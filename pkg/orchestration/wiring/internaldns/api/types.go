package apiinternaldns

import "github.com/atlassian/voyager"

const (
	ResourceType voyager.ResourceType = "InternalDNS"
)

type Alias struct {
	AliasType string `json:"type"`
	Name      string `json:"name"`
}

type Spec struct {
	Aliases []Alias `json:"aliases"`
}
