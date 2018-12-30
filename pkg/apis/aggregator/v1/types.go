package v1

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/aggregator"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const (
	AggregateResourceSingular = "aggregate"
	AggregateResourcePlural   = "aggregate"
	AggregateResourceVersion  = "v1"
	AggregateResourceKind     = "Aggregate"

	AggregateResourceAPIVersion = aggregator.GroupName + "/" + AggregateResourceVersion
)

var (
	AggregateGvk = SchemeGroupVersion.WithKind(AggregateResourceKind)
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Aggregate struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	voyager.Location   `json:",inline"`
	Name               string      `json:"name"`
	Body               interface{} `json:"body,omitempty"`
	StatusCode         int         `json:"statusCode"`
	Error              *string     `json:"error,omitempty"`
}

// AggregateList is a list of Aggregates.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AggregateList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Aggregate `json:"items"`
}

func (in *Aggregate) DeepCopyInto(out *Aggregate) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Body = runtime.DeepCopyJSONValue(in.Body)
	if in.Error != nil {
		err := *in.Error
		out.Error = &err
	}
}
