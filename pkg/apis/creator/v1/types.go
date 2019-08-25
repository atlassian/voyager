package v1

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/creator"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ServiceResourceSingular = "service"
	ServiceResourcePlural   = "services"
	ServiceResourceVersion  = "v1"
	ServiceResourceKind     = "Service"
	ServiceListResourceKind = "ServiceList"

	ServiceResourceAPIVersion = creator.GroupName + "/" + ServiceResourceVersion

	ServiceResourceName = ServiceResourcePlural + "." + creator.GroupName
)

var (
	ServiceGvk = SchemeGroupVersion.WithKind(ServiceResourceKind)
)

// +genclient
// +genclient:nonNamespaced
// +genclient:noStatus
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Service struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type ServiceSpec struct {
	BusinessUnit       string                 `json:"businessUnit,omitempty"`
	ResourceOwner      string                 `json:"resourceOwner,omitempty"`
	SSAMContainerName  string                 `json:"ssamContainerName,omitempty"`
	PagerDutyServiceID string                 `json:"pagerDutyServiceID,omitempty"`
	LoggingID          string                 `json:"loggingID,omitempty"`
	Metadata           ServiceMetadata        `json:"metadata,omitempty"`
	ResourceTags       map[voyager.Tag]string `json:"tags,omitempty"`
}

// +k8s:deepcopy-gen=true
type ServiceStatus struct {
	Compliance Compliance `json:"compliance,omitempty"`
}

// EmailAddress gives the email address for the service
func (ss *ServiceSpec) EmailAddress() string {
	return ss.ResourceOwner + "@atlassian.com"
}

// +k8s:deepcopy-gen=true
type ServiceMetadata struct {
	PagerDuty *PagerDutyMetadata `json:"pagerDuty,omitempty"`
	Opsgenie  *OpsgenieMetadata  `json:"opsgenie,omitempty"`
	Bamboo    *BambooMetadata    `json:"bamboo,omitempty"`
}

// +k8s:deepcopy-gen=true
type BambooMetadata struct {
	Builds      []BambooPlanRef `json:"builds,omitempty"`
	Deployments []BambooPlanRef `json:"deployments,omitempty"`
}

type BambooPlanRef struct {
	Server string `json:"server"`
	Plan   string `json:"plan"`
}

type PagerDutyMetadata struct {
	Staging    PagerDutyEnvMetadata `json:"staging,omitempty"`
	Production PagerDutyEnvMetadata `json:"production,omitempty"`
}

type PagerDutyEnvMetadata struct {
	Main        PagerDutyServiceMetadata `json:"main,omitempty"`
	LowPriority PagerDutyServiceMetadata `json:"lowPriority,omitempty"`
}

type PagerDutyServiceMetadata struct {
	ServiceID    string                `json:"serviceID,omitempty"`
	PolicyID     string                `json:"policyID,omitempty"`
	Integrations PagerDutyIntegrations `json:"integrations,omitempty"`
}

type PagerDutyIntegrations struct {
	CloudWatch PagerDutyIntegrationMetadata `json:"cloudWatch,omitempty"`
	Generic    PagerDutyIntegrationMetadata `json:"generic,omitempty"`
	Pingdom    PagerDutyIntegrationMetadata `json:"pingdom,omitempty"`
}

type PagerDutyIntegrationMetadata struct {
	IntegrationID  string `json:"integrationID,omitempty"`
	IntegrationKey string `json:"integrationKey,omitempty"`
}

type OpsgenieMetadata struct {
	Team string `json:"team,omitempty"`
}

// +k8s:deepcopy-gen=true
type Compliance struct {
	PRGBControl *bool `json:"prgbControl,omitempty"`
}

// ServiceList is a list of Services.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Service `json:"items"`
}
