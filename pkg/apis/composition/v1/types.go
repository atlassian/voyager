package v1

import (
	"encoding/json"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/composition"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ServiceDescriptorResourceSingular = "servicedescriptor"
	ServiceDescriptorResourcePlural   = "servicedescriptors"
	ServiceDescriptorResourceVersion  = "v1"
	ServiceDescriptorResourceKind     = "ServiceDescriptor"
	ServiceDescriptorResourceListKind = ServiceDescriptorResourceKind + "List"

	ServiceDescriptorResourceName = ServiceDescriptorResourcePlural + "." + composition.GroupName
)

var (
	ServiceDescriptorGVK = SchemeGroupVersion.WithKind(ServiceDescriptorResourceKind)
)

type Scope string

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced

// ServiceDescriptor describes the architecture of a service
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceDescriptor struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec ServiceDescriptorSpec `json:"spec"`

	Status ServiceDescriptorStatus `json:"status,omitempty"`
}

// ServiceDescriptorSpec is the top-level definition for a service's architecture
// +k8s:deepcopy-gen=true
type ServiceDescriptorSpec struct {
	Locations      []ServiceDescriptorLocation      `json:"locations"`
	Config         []ServiceDescriptorConfigSet     `json:"config,omitempty"`
	ResourceGroups []ServiceDescriptorResourceGroup `json:"resourceGroups,omitempty"`
	Version        string                           `json:"version"`
}

type ServiceDescriptorLocationName string
type ServiceDescriptorResourceGroupName string

// ServiceDescriptorLocation describes a distinct voyager location to deploy to, with a user-supplied name for that location
// +k8s:deepcopy-gen=true
type ServiceDescriptorLocation struct {
	Name    ServiceDescriptorLocationName `json:"name"`
	Account voyager.Account               `json:"account,omitempty"`
	Region  voyager.Region                `json:"region"`
	EnvType voyager.EnvType               `json:"envType"`
	Label   voyager.Label                 `json:"label,omitempty"`
}

func (l ServiceDescriptorLocation) VoyagerLocation() voyager.Location {
	return voyager.Location{
		Account: l.Account,
		Region:  l.Region,
		EnvType: l.EnvType,
		Label:   l.Label,
	}
}

// ServiceDescriptorConfigSet is a map of variable names & values
// +k8s:deepcopy-gen=true
type ServiceDescriptorConfigSet struct {
	Scope Scope                  `json:"scope"`
	Vars  map[string]interface{} `json:"vars"`
}

// DeepCopyInto handles the copying of the JSON attributes specified as interface{}
func (in *ServiceDescriptorConfigSet) DeepCopyInto(out *ServiceDescriptorConfigSet) {
	*out = *in
	out.Vars = runtime.DeepCopyJSON(in.Vars)
}

// ServiceDescriptorResourceGroup describes a set of resources that exist in one or more locations
// The Locations map back to the locations in the top-level of the Spec
// +k8s:deepcopy-gen=true
type ServiceDescriptorResourceGroup struct {
	Name      ServiceDescriptorResourceGroupName `json:"name"`
	Locations []ServiceDescriptorLocationName    `json:"locations"`
	Resources []ServiceDescriptorResource        `json:"resources"`
}

// ServiceDescriptorResourceDependency describes the dependency of one resources on another, with optional attributes
// +k8s:deepcopy-gen=true
type ServiceDescriptorResourceDependency struct {
	Name       voyager.ResourceName   `json:"name"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (in *ServiceDescriptorResourceDependency) DeepCopyInto(out *ServiceDescriptorResourceDependency) {
	*out = *in
	out.Attributes = runtime.DeepCopyJSON(in.Attributes)
}

// UnmarshalJSON for ServiceDescriptorResourceDependency handles them being either a single
// resource name, or an object containing name and attributes.
func (in *ServiceDescriptorResourceDependency) UnmarshalJSON(data []byte) error {
	type fakeServiceDescriptorResourceDependency ServiceDescriptorResourceDependency
	var res fakeServiceDescriptorResourceDependency
	if err := json.Unmarshal(data, &res); err == nil {
		in.Name = res.Name
		in.Attributes = res.Attributes
		return nil
	}

	var resourceName voyager.ResourceName
	if err := json.Unmarshal(data, &resourceName); err != nil {
		return err
	}

	in.Name = resourceName
	return nil
}

// ServiceDescriptorResource describes the a voyager resource
// +k8s:deepcopy-gen=true
type ServiceDescriptorResource struct {
	Name voyager.ResourceName `json:"name,omitempty"`
	Type voyager.ResourceType `json:"type,omitempty"`

	DependsOn []ServiceDescriptorResourceDependency `json:"dependsOn,omitempty"`

	// Specification of the desired behavior of the Resource.
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// ServiceDescriptorList is a list of ServiceDescriptors.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceDescriptorList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []ServiceDescriptor `json:"items"`
}

// +k8s:deepcopy-gen=true
type ServiceDescriptorStatus struct {
	Conditions       []cond_v1.Condition `json:"conditions,omitempty"`
	LocationStatuses []LocationStatus    `json:"locationStatuses"`
}

// +k8s:deepcopy-gen=true
type LocationStatus struct {
	DescriptorName      string              `json:"descriptorName"`
	DescriptorNamespace string              `json:"descriptorNamespace"`
	Conditions          []cond_v1.Condition `json:"conditions"`
	Location            voyager.Location    `json:"location"`
}
