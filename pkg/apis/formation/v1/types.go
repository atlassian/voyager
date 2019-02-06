package v1

import (
	"bytes"
	"encoding/json"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/formation"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	LocationDescriptorResourceSingular = "locationdescriptor"
	LocationDescriptorResourcePlural   = "locationdescriptors"
	LocationDescriptorResourceVersion  = "v1"
	LocationDescriptorResourceKind     = "LocationDescriptor"

	LocationDescriptorResourceAPIVersion = formation.GroupName + "/" + LocationDescriptorResourceVersion

	LocationDescriptorResourceName = LocationDescriptorResourcePlural + "." + formation.GroupName
)

// +genclient
// +genclient:noStatus

// LocationDescriptor is handled by LocationDescriptor controller.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LocationDescriptor struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the LocationDescriptor.
	Spec LocationDescriptorSpec `json:"spec,omitempty"`

	// Most recently observed status of the LocationDescriptor.
	Status LocationDescriptorStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type LocationDescriptorSpec struct {
	// ConfigMapName is the name of the config map to read in the LocationDescriptor's
	// namespace that will hold the LocationDescriptor configuration, e.g. tags,
	// businessUnit, notificationEmail
	ConfigMapName string `json:"configMapName"` // TODO: rename and move this to the ConfigMaps structure below

	ConfigMapNames LocationDescriptorConfigMapNames `json:"configMapNames"`

	// Resources is a list of resources that will be autowired.
	Resources []LocationDescriptorResource `json:"resources,omitempty"`
}

// +k8s:deepcopy-gen=true
type LocationDescriptorConfigMapNames struct {
	// The name of the ConfigMap containing the "release" data for variable expansion in formation processing
	Release string `json:"release"`
}

// +k8s:deepcopy-gen=true
type LocationDescriptorResource struct {
	Name voyager.ResourceName `json:"name"`
	Type voyager.ResourceType `json:"type"`
	// Explicit dependencies.
	DependsOn []LocationDescriptorDependency `json:"dependsOn,omitempty"`
	// Specification of the desired behavior of the Resource.
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen=true
type LocationDescriptorDependency struct {
	Name       voyager.ResourceName   `json:"name"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (ldd *LocationDescriptorDependency) DeepCopyInto(out *LocationDescriptorDependency) {
	*out = *ldd
	out.Attributes = runtime.DeepCopyJSON(ldd.Attributes)
}

// UnmarshalJSON for LocationDescriptorDependency handles them being either a single
// resource name, or an object containing name and attributes.
func (ldd *LocationDescriptorDependency) UnmarshalJSON(data []byte) error {
	type fakeLocationDescriptorDependency LocationDescriptorDependency
	var res fakeLocationDescriptorDependency
	if err := json.Unmarshal(data, &res); err == nil {
		ldd.Name = res.Name
		ldd.Attributes = res.Attributes
		return nil
	}

	var resourceName voyager.ResourceName
	if err := json.Unmarshal(data, &resourceName); err != nil {
		return err
	}

	ldd.Name = resourceName
	return nil
}

// SpecIntoTyped converts Raw representation of the object into a typed representation.
func (ldr *LocationDescriptorResource) SpecIntoTyped(obj interface{}) error {
	if ldr.Spec == nil {
		return errors.New("spec is null")
	}
	return json.Unmarshal(ldr.Spec.Raw, obj)
}

// +k8s:deepcopy-gen=true
type LocationDescriptorStatus struct {
	// Represents the latest available observations of a LocationDescriptor's current LocationDescriptor.
	Conditions       []cond_v1.Condition `json:"conditions,omitempty"`
	ResourceStatuses []ResourceStatus    `json:"resourceStatuses,omitempty"`
}

// +k8s:deepcopy-gen=true
type ResourceStatus struct {
	Name       voyager.ResourceName `json:"name,omitempty"`
	Conditions []cond_v1.Condition  `json:"conditions,omitempty"`
}

func (lds *LocationDescriptorStatus) String() string {
	first := true
	var buf bytes.Buffer
	buf.WriteByte('[') // nolint: gosec
	for _, cond := range lds.Conditions {
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

// LocationDescriptorList is a list of LocationDescriptors.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LocationDescriptorList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []LocationDescriptor `json:"items"`
}
