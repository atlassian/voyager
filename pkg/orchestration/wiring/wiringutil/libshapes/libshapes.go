package libshapes

import (
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

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

// BindableShapeStruct represents a bit of information that is needed to create a Service Catalog ServiceBinding
// object. To be embedded into other shapes' structs where a ServiceInstance needs to be bound to to get outputs
// for that shape.
// If an autowiring plugin exposes multiple shapes that have this struct embedded it may or may not be the case
// that they all refer to the same ServiceInstance. It is responsibility of the consuming side to track if more than
// one ServiceBinding needs to be created to consume values from those shapes.
// +k8s:deepcopy-gen=true
type BindableShapeStruct struct {
	ServiceInstanceName ProtoReference `json:"serviceInstanceName"`
}

// FindAndCopyShapeByName iterates over a given array of Shapes, finding one based on a given name and will error if the given name belongs to multiple shapes
func FindAndCopyShapeByName(shapes []wiringplugin.Shape, name wiringplugin.ShapeName, copyInto wiringplugin.Shape) (bool /*found*/, error) {
	found := false
	for _, shape := range shapes {
		if shape.Name() == name {
			// Ensure we only have one of the same shape
			if found {
				return found, fmt.Errorf("found multiple shapes with name %s", name)
			}
			found = true
			err := wiringplugin.CopyShape(shape, copyInto)
			if err != nil {
				return found, errors.Wrapf(err, "failed to copy shape %T into %T", shape, copyInto)
			}
		}
	}
	return found, nil
}
