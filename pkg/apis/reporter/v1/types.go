package v1

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/reporter"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ReportResourceSingular = "report"
	ReportResourcePlural   = "reports"
	ReportResourceVersion  = "v1"
	ReportResourceKind     = "Report"

	SummaryResourceSingular = "summary"
	SummaryResourcePlural   = "summaries"
	SummaryResourceVersion  = "v1"
	SummaryResourceKind     = "Summary"

	ReportResourceAPIVersion = reporter.GroupName + "/" + ReportResourceVersion

	RouteResourceName = ReportResourcePlural + "." + reporter.GroupName

	LayerComposition   = "composition"
	LayerFormation     = "formation"
	LayerOrchestration = "orchestration"
	LayerExecution     = "execution"
	LayerObject        = "object"
	LayerProvider      = "provider"
)

var (
	ReportGvk = SchemeGroupVersion.WithKind(ReportResourceKind)
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Report struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	voyager.Location   `json:",inline"`
	Report             NamespaceReport `json:"report"`
}

// ReportList is a list of Reports.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ReportList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Report `json:"items"`
}

// +k8s:deepcopy-gen=true
type Status struct {
	Status    string        `json:"status,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Timestamp *meta_v1.Time `json:"timestamp,omitempty"`
}

// +k8s:deepcopy-gen=true
type Reference struct {
	Name         string                 `json:"name"`
	Layer        string                 `json:"layer"`
	ResourceType string                 `json:"type,omitempty"`
	Attributes   map[string]interface{} `json:"attributes,omitempty"`
}

func (in *Reference) DeepCopyInto(out *Reference) {
	*out = *in
	out.Attributes = runtime.DeepCopyJSON(in.Attributes)
}

type ResourceProvider struct {
	PlanID     string `json:"planId"`
	ClassID    string `json:"classId"`
	Namespaced bool   `json:"namespaced"`
}

// Event is a cut-down version of the real Kubernetes Event object
// For the user, it may be better to present something like:
//   https://github.com/kubernetes/kubernetes/blob/8c6fbd708c1012c397eb9c746a89c4fc907f02d4/pkg/printers/internalversion/describe.go#L3195
//
// +k8s:deepcopy-gen=true
type Event struct {
	Reason         string            `json:"reason,omitempty"`
	Message        string            `json:"message,omitempty"`
	FirstTimestamp meta_v1.Time      `json:"firstTimestamp,omitempty"`
	LastTimestamp  meta_v1.Time      `json:"lastTimestamp,omitempty"`
	Count          int32             `json:"count,omitempty"`
	Type           string            `json:"type,omitempty"`
	EventTime      meta_v1.MicroTime `json:"eventTime,omitempty"`
}

func ConvertV1EventToEvent(event *core_v1.Event) *Event {
	return &Event{
		Reason:         event.Reason,
		Message:        event.Message,
		FirstTimestamp: event.FirstTimestamp,
		LastTimestamp:  event.LastTimestamp,
		Count:          event.Count,
		Type:           event.Type,
		EventTime:      event.EventTime,
	}
}

// +k8s:deepcopy-gen=true
type Resource struct {
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	ResourceType string            `json:"type,omitempty"`
	Properties   interface{}       `json:"properties,omitempty"`
	Spec         interface{}       `json:"spec,omitempty"`
	Status       Status            `json:"status,omitempty"`
	References   []Reference       `json:"references,omitempty"`
	UID          string            `json:"uid,omitempty"`
	Provider     *ResourceProvider `json:"provider,omitempty"`
	Events       []Event           `json:"events,omitempty"`
}

func (in *Resource) DeepCopyInto(out *Resource) {
	*out = *in
	out.Properties = runtime.DeepCopyJSONValue(in.Properties)
	out.Spec = runtime.DeepCopyJSONValue(in.Spec)

	in.Status.DeepCopyInto(&out.Status)
	if in.References != nil {
		in, out := &in.References, &out.References
		*out = make([]Reference, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// +k8s:deepcopy-gen=true
type ReportLayer struct {
	Resources []Resource `json:"resources"`
	Status    Status     `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type NamespaceReport struct {
	Composition   ReportLayer `json:"composition"`
	Formation     ReportLayer `json:"formation"`
	Orchestration ReportLayer `json:"orchestration"`
	Execution     ReportLayer `json:"execution"`
	Objects       ReportLayer `json:"objects"`
	Providers     ReportLayer `json:"providers"`
}
