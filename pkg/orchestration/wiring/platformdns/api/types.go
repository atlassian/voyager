package apiplatformdns

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/servicecatalog"
)

const (
	ResourceType                  voyager.ResourceType           = "PlatformDNS"
	AliasTypeSimple               string                         = "Simple"
	ClusterServiceClassExternalID servicecatalog.ClassExternalID = "f77e1881-36f3-42ce-9848-7a811b421dd7"
	ClusterServicePlanExternalID  servicecatalog.PlanExternalID  = "0a7b1d18-cf8d-461e-ad24-ee16d3da36d3"
)

type Spec struct {
	AliasType string `json:"type"`
	Name      string `json:"name"`
}
