package servicecentral

import (
	"time"
)

// ServiceName is the name of a Service.
// Not voyager.ServiceName because not all Services in Service Central are Voyager services.
// I.e. set of all voyager.ServiceName is a subset of all Service Central ServiceNames.
type ServiceName string

type ServiceOwner struct {
	Username string `json:"username"`
}

type ServiceData struct {
	ServiceUUID        *string      `json:"service_uuid,omitempty"`
	CreationTimestamp  *string      `json:"creation_timestamp,omitempty"`
	ServiceName        ServiceName  `json:"service_name,omitempty"`
	ServiceOwner       ServiceOwner `json:"service_owner,omitempty"`
	ServiceTier        int          `json:"service_tier,omitempty"`
	Tags               []string     `json:"tags,omitempty"`
	Platform           string       `json:"platform,omitempty"`
	Misc               []miscData   `json:"misc,omitempty"`
	PagerDutyServiceID string       `json:"pagerduty_service_id,omitempty"`
	LoggingID          string       `json:"logging_id,omitempty"`
	SSAMContainerName  string       `json:"ssam_container_name,omitempty"`

	ZeroDowntimeUpgrades bool   `json:"zero_downtime_upgrades,omitempty"`
	Stateless            bool   `json:"stateless,omitempty"`
	BusinessUnit         string `json:"business_unit,omitempty"`

	Attributes []ServiceAttribute

	// Compliance is a read-only field. It can be nil, in which case it means
	// they have not completed their compliance questions yet
	Compliance *ServiceComplianceConf `json:"compliance,omitempty"`
}

type ServiceAttribute struct {
	Team string
}

type serviceAttributeResponse struct {
	ID      int `json:"id"`
	Service struct {
		Ref  string `json:"ref"`
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"service"`
	Schema struct {
		Ref  string `json:"ref"`
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"schema"`
	Value      map[string]string `json:"value"`
	CreatedOn  time.Time         `json:"createdOn"`
	CreatedBy  string            `json:"createdBy"`
	ModifiedOn time.Time         `json:"modifiedOn"`
	ModifiedBy string            `json:"modifiedBy"`
}

// ServiceComplianceConf includes all service compliance related data
type ServiceComplianceConf struct {
	// using prgb_control instead of prgbControl here to match the response field name
	PRGBControl *bool `json:"prgb_control,omitempty"`
}

// most of these fields are optional
type metaResult struct {
	CurrentResultCount *int    `json:"current_result_count"`
	Limit              *int    `json:"limit"`
	Next               *string `json:"next"`
	Offset             *int    `json:"offset"`
	Previous           *string `json:"previous"`

	// may be empty
	QueryArgs map[string]interface{} `json:"query_args"`
}

// Return value representing service and associated arbitrary metadata
type serviceTypeResponse struct {
	Data       []ServiceData `json:"data"`
	Message    string        `json:"message"`
	StatusCode int           `json:"status_code"`

	// only useful for search list results
	Meta metaResult `json:"meta"`
}

type miscData struct {
	Key   string `json:"misc_key,omitempty"`
	Value string `json:"misc_value,omitempty"`
	// note that SC does have a `uuid` field for misc, but we intentionally exclude it
	// if we do not exclude it, then updates to existing fields will not work
}

// Copied from https://stash.atlassian.com/projects/MICROS/repos/central/browse/pkg/central/model/service.go
// Do not modify!
type V2Service struct {
	tableName struct{} `json:"-" sql:"services,alias:service" pg:",discard_unknown_columns"` // nolint

	// Unique identifier of a service
	// read only: true
	UUID string `json:"uuid" sql:"service_uuid,pk"`

	// Name of the services, must be unique
	Name string `json:"name" sql:"service_name,unique"`

	// Owner of the service, must be an a valid staff ID of a person
	Owner string `json:"owner" sql:"service_owner"`

	// Name of the team who owns the service
	Team *string `json:"team" sql:"team"`

	// Name of the business unit which owns the service
	BusinessUnit *string `json:"businessUnit" sql:"business_unit"`

	// SSAM Container attached to this service
	SSAMContainerName *string `json:"ssamContainerName" sql:"ssam_container_name"`

	// Location of the service - not sure what this means... physical location?
	Location *string `json:"location" sql:"location"`

	// Name of the platform on which the service runs (micros, vm, etc)
	Platform *string `json:"platform" sql:"platform"`

	// Description of the service
	Description *string `json:"description" sql:"description"`

	// URL of the service - which doesn't make alot of sense if it is a multi region/environment service
	ServiceURL *string `json:"servingURL" sql:"serving_url"`

	// Name of the Hipchat / Stride room where you can reach the team that maintains the service
	HipChatRoomName *string `json:"hipchatRoomName" sql:"hipchat_room_name"`

	// Tier of the service, must be one of: 0, 1, 2, 3
	ServiceTier *int `json:"serviceTier" sql:"service_tier"`

	// Unique identifier in the logging platform
	LoggingID *string `json:"loggingID" sql:"logging_id"`

	// ID of compliance policy attached to this service
	CompliancePolicyID *int `json:"compliancePolicyID" sql:"compliance_policy_id"`

	// ID of the Pagerduty service attached to this service
	PagerDutyServiceID *string `json:"pagerdutyServiceID" sql:"pagerduty_service_id"`

	// Is this service public facing?
	PublicFacing *bool `json:"publicFacing" sql:"public_facing"`

	// Is this service provided by a third party?
	ThirdPartyProvided *bool `json:"thirdPartyProvided" sql:"third_party_provided"`

	// Is the service stateless?
	Stateless *bool `json:"stateless" sql:"stateless"`

	// Is the service guaranteeing zero downtime upgrades?
	ZeroDowntimeUpgrades *bool `json:"zeroDowntimeUpgrades" sql:"zero_downtime_upgrades"`

	// Timestamp for when the record was created
	// read only: true
	CreatedOn time.Time `json:"createdOn" sql:"creation_timestamp"`

	// Timestamp for when the record was last modified
	// read only: true
	ModifiedOn time.Time `json:"modifiedOn" sql:"last_modified_timestamp"`

	// User who last modified the record
	// read only: true
	ModifiedBy string `json:"modifiedBy" sql:"last_modified_user"`
}
