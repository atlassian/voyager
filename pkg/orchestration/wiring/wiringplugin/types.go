// Package wiringplugin provides the wiring-related types surrounding "WiringPlugin"
package wiringplugin

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// WiringPlugin represents an autowiring plugin.
// Autowiring plugin is an in-code representation of an autowiring function.
type WiringPlugin interface {
	// WireUp wires up the resource.
	// Error may be retriable if its an RPC error (like network error). Most errors are not retriable because
	// this method should be pure/deterministic so if it fails, it fails.
	WireUp(resource *orch_v1.StateResource, context *WiringContext) (*WiringResult, bool /*retriable*/, error)
}

// WiringContext contains context information that is passed to an autowiring function to perform autowiring
// for a resource.
type WiringContext struct {
	StateMeta    meta_v1.ObjectMeta
	StateContext StateContext
	Dependencies []WiredDependency
	Dependants   []DependantResource
}

// WiredDependency represents a resource that has been processed by a corresponding autowiring function.
type WiredDependency struct {
	Name     voyager.ResourceName
	Type     voyager.ResourceType
	Contract ResourceContract
	// DEPRECATED: use Contract
	SmithResources []smith_v1.Resource
	// Attributes are attributes attached to the edge between resources.
	Attributes map[string]interface{}
}

// DependantResource represents a resource that depends on the resource that is currently being processed.
type DependantResource struct {
	Name voyager.ResourceName
	Type voyager.ResourceType
	// Attributes are attributes attached to the edge between resources.
	Attributes map[string]interface{}
	Resource   orch_v1.StateResource
}

// ProtoReferenceName is the name of a proto reference.
type ProtoReferenceName string

// ProtoReference represents bits of information that need to be augmented with more information to
// construct a valid Smith reference.
// +k8s:deepcopy-gen=true
type ProtoReference struct {
	Resource smith_v1.ResourceName `json:"resource"`
	Path     string                `json:"path,omitempty"`
	Example  interface{}           `json:"example,omitempty"`
	Modifier string                `json:"modifier,omitempty"`
}

// ToReference should be used to augment ProtoReference with missing information to
// get a full Reference.
func (r *ProtoReference) ToReference(name smith_v1.ReferenceName) smith_v1.Reference {
	return smith_v1.Reference{
		Name:     name,
		Resource: r.Resource,
		Path:     r.Path,
		Example:  r.Example,
		Modifier: r.Modifier,
	}
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (r *ProtoReference) DeepCopyInto(out *ProtoReference) {
	*out = *r
	out.Example = runtime.DeepCopyJSONValue(r.Example)
}

// NamedProtoReference is a ProtoReference that has a name.
// That name is typically used to find the needed proto reference in a list/map.
type NamedProtoReference struct {
	Name           ProtoReferenceName `json:"name"`
	ProtoReference `json:",inline"`
}

// BindingProtoReference is a reference to the ServiceBinding's contents.
// +k8s:deepcopy-gen=true
type BindingProtoReference struct {
	Path    string      `json:"path,omitempty"`
	Example interface{} `json:"example,omitempty"`
}

func (r *BindingProtoReference) DeepCopyInto(out *BindingProtoReference) {
	*out = *r
	out.Example = runtime.DeepCopyJSONValue(r.Example)
}

// ToReference should be used to augment BindingProtoReference with missing information to
// get a full Reference.
func (r *BindingProtoReference) ToReference(name smith_v1.ReferenceName, bindingResourceName smith_v1.ResourceName) smith_v1.Reference {
	return smith_v1.Reference{
		Name:     name,
		Resource: bindingResourceName,
		Path:     r.Path,
		Example:  r.Example,
	}
}

// BindingProtoReference is a reference to the ServiceBinding's Secret's contents.
// +k8s:deepcopy-gen=true
type BindingSecretProtoReference struct {
	Path    string      `json:"path,omitempty"`
	Example interface{} `json:"example,omitempty"`
}

func (r *BindingSecretProtoReference) DeepCopyInto(out *BindingSecretProtoReference) {
	*out = *r
	out.Example = runtime.DeepCopyJSONValue(r.Example)
}

// ToReference should be used to augment BindingSecretProtoReference with missing information to
// get a full Reference.
func (r *BindingSecretProtoReference) ToReference(name smith_v1.ReferenceName, bindingResourceName smith_v1.ResourceName) smith_v1.Reference {
	return smith_v1.Reference{
		Name:     name,
		Resource: bindingResourceName,
		Path:     r.Path,
		Example:  r.Example,
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
}

// ResourceContract contains information about a resource for consumption by other autowiring functions.
// It is the API of a resource that can be depended upon and hence should not change unexpectedly without
// a proper migration path to a new version.
type ResourceContract struct {
	Shapes []Shape               `json:"shapes,omitempty"`
	Refs   []NamedProtoReference `json:"refs,omitempty"`
	Data   []DataItem            `json:"data,omitempty"`
}

func (c *ResourceContract) FindShape(shapeName ShapeName) (Shape, bool /* found */) {
	for _, shape := range c.Shapes {
		if shape.Name() == shapeName {
			return shape, true
		}
	}

	return nil, false
}

func (c *ResourceContract) IsEmpty() bool {
	return len(c.Shapes) == 0 && len(c.Refs) == 0 && len(c.Data) == 0
}

// DataItem is a named bit of data made available by an autowiring function.
type DataItem struct {
	Name string
	// Data is the data for this item.
	// Only contains types produced by json.Unmarshal() and also int64:
	// bool, int64, float64, string, []interface{}, map[string]interface{}, json.Number and nil
	Data interface{}
}

type WiringResult struct {
	Contract  ResourceContract
	Resources []WiredSmithResource
}

type WiredSmithResource struct {
	SmithResource smith_v1.Resource
	// DEPRECATED: use Shapes, Refs and/or Data in ResourceContract to expose information
	Exposed bool
}

// StateContext is used as input for the plugins. Everything in the StateContext
// is constructed from a combination of the Entangler struct, the State resource,
// and the EntanglerContext.
// This has a few legacy concepts tied to Atlassian which we could probably move
// to being read from user-provided autowiring functions.
type StateContext struct {
	// Location is constructed from a combination of ClusterLocation and the label
	// from the EntanglerContext.
	Location voyager.Location

	// LegacyConfig is read by a function specified in the entangler struct.
	// TODO this is a temporary container for 'stuff that's in Micros config.js'.
	// It needs to be migrated ... somewhere. Either to the providers, the cluster
	// config, a configuration file, ...
	LegacyConfig legacy.Config

	ServiceName voyager.ServiceName

	// ServiceProperties is extra metadata we pulled from the EntanglerContext
	// which comes from a ConfigMap tied to the State.
	ServiceProperties orch_meta.ServiceProperties

	// Tags is the final computed tags that include business_unit and service_name
	// and etc.
	Tags map[voyager.Tag]string

	// ClusterConfig is the cluster config.
	ClusterConfig ClusterConfig
}

type ClusterConfig struct {
	// ClusterDomainName is the domain name of the ingress.
	ClusterDomainName string
	KittClusterEnv    string
	Kube2iamAccount   string
}
