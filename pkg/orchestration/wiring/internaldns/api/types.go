package apiinternaldns

import "github.com/atlassian/voyager"

const (
	ResourceType                  voyager.ResourceType = "InternalDNS"
	AliasTypeSimple               string               = "Simple"
	ClusterServiceClassExternalID                      = "f77e1881-36f3-42ce-9848-7a811b421dd7"
	ClusterServicePlanExternalID                       = "0a7b1d18-cf8d-461e-ad24-ee16d3da36d3"
)

type Alias struct {
	AliasType string `json:"type"`
	Name      string `json:"name"`
}

type Spec struct {
	Aliases []Alias `json:"aliases"`
}
