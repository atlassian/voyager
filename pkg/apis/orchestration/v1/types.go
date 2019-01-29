package v1

import (
	"bytes"
	"encoding/json"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/orchestration"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	StateResourceSingular = "state"
	StateResourcePlural   = "states"
	StateResourceVersion  = "v1"
	StateResourceKind     = "State"

	StateResourceAPIVersion = orchestration.GroupName + "/" + StateResourceVersion

	StateResourceName = StateResourcePlural + "." + orchestration.GroupName
)

// +genclient
// +genclient:noStatus

// State is handled by State controller.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type State struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the State.
	Spec StateSpec `json:"spec,omitempty"`

	// Most recently observed status of the State.
	Status StateStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type StateSpec struct {
	//Name      string          `json:"name,omitempty"` // TODO this field exists in examples, but do we actually need it?

	// ConfigMapName is the name of the config map to read in the State's
	// namespace that will hold the state configuration, e.g. tags,
	// businessUnit, notificationEmail
	ConfigMapName string `json:"configMapName"`

	// Resources is a list of resources that will be autowired.
	Resources []StateResource `json:"resources,omitempty"`
}

// +k8s:deepcopy-gen=true
type StateResource struct {
	Name voyager.ResourceName `json:"name"`
	Type voyager.ResourceType `json:"type"`
	// Explicit dependencies.
	DependsOn []StateDependency     `json:"dependsOn,omitempty"`
	Defaults  *runtime.RawExtension `json:"defaults,omitempty"`
	// Specification of the desired behavior of the Resource.
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen=true
type StateDependency struct {
	Name       voyager.ResourceName   `json:"name"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (sd *StateDependency) DeepCopyInto(out *StateDependency) {
	*out = *sd

	// Fixed in apimachinery 1.11 but without this a nil attributes field fails reflect.DeepEqual()
	// See https://github.com/kubernetes/kubernetes/commit/53e8fd04ec39034b28d3a13920548e9761f6ac70
	if sd.Attributes == nil {
		out.Attributes = sd.Attributes
	} else {
		out.Attributes = runtime.DeepCopyJSON(sd.Attributes)
	}
}

// UnmarshalJSON for StateDependency handles them being either a single
// resource name, or an object containing name and attributes.
func (sd *StateDependency) UnmarshalJSON(data []byte) error {
	type fakeStateDependency StateDependency
	res := fakeStateDependency{}
	if err := json.Unmarshal(data, &res); err == nil {
		sd.Name = res.Name
		sd.Attributes = res.Attributes
		return nil
	}

	var resourceName voyager.ResourceName
	if err := json.Unmarshal(data, &resourceName); err != nil {
		return err
	}

	sd.Name = resourceName
	// TODO: the empty map below (vs nil) is necessary due to deepcopy issues. Remove once we are on 1.11 machinery
	// see also  https://github.com/kubernetes/kubernetes/pull/62063/files#diff-42b29a4397bd0cf0a0e6183239fbe393R448
	// https://github.com/kubernetes/kubernetes/pull/62063#discussion_r179122869
	sd.Attributes = map[string]interface{}{}
	return nil
}

// SpecIntoTyped converts Raw representation of the object into a typed representation.
func (sr *StateResource) SpecIntoTyped(obj interface{}) error {
	if sr.Spec == nil {
		return errors.New("spec is null")
	}
	return json.Unmarshal(sr.Spec.Raw, obj)
}

// +k8s:deepcopy-gen=true
type StateStatus struct {
	// Represents the latest available observations of a State's current state.
	Conditions       []cond_v1.Condition `json:"conditions,omitempty"`
	ResourceStatuses []ResourceStatus    `json:"resourceStatuses,omitempty"`
}

func (ss *StateStatus) String() string {
	first := true
	var buf bytes.Buffer
	buf.WriteByte('[') // nolint: gosec
	for _, cond := range ss.Conditions {
		if first {
			first = false
		} else {
			buf.WriteByte('|') // nolint: gosec
		}
		buf.WriteString(cond.String()) // nolint: gosec
	}
	buf.WriteByte(']') // nolint: gosec
	return buf.String()
}

// +k8s:deepcopy-gen=true
type ResourceStatusData struct {
	Conditions []cond_v1.Condition    `json:"conditions,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (in *ResourceStatusData) DeepCopyInto(out *ResourceStatusData) {
	*out = *in

	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]cond_v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.Data = runtime.DeepCopyJSON(in.Data)
}

// +k8s:deepcopy-gen=true
type ResourceStatus struct {
	Name               voyager.ResourceName `json:"name,omitempty"`
	ResourceStatusData `json:",inline"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (in *ResourceStatus) DeepCopyInto(out *ResourceStatus) {
	*out = *in

	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]cond_v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.Data = runtime.DeepCopyJSON(in.Data)
}

func (s *State) GetCondition(conditionType cond_v1.ConditionType) (int, *cond_v1.Condition) {
	for i, condition := range s.Status.Conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}

// StateList is a list of States.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type StateList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []State `json:"items"`
}
