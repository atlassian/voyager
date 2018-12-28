package v1

import (
	"github.com/atlassian/voyager/pkg/apis/ops"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RouteResourceSingular  = "route"
	RouteResourcePlural    = "routes"
	RouteResourceVersion   = "v1"
	RouteResourceKind      = "Route"
	RouteResourceShortName = "rt"

	RouteResourceAPIVersion = ops.GroupName + "/" + RouteResourceVersion

	RouteResourceName = RouteResourcePlural + "." + ops.GroupName
)

var (
	RouteGvk = SchemeGroupVersion.WithKind(RouteResourceKind)
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Route struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the State.
	Spec RouteSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen=true
type RouteSpec struct {
	URL   string   `json:"url"`
	ASAP  ASAPSpec `json:"asap"`
	Plans []string `json:"plans"`
}

type ASAPSpec struct {
	Audience string `json:"audience"`
}

// RouteList is a list of Routes.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RouteList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Route `json:"items"`
}
