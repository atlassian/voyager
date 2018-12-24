package wiringplugin

import "k8s.io/apimachinery/pkg/runtime"

// ShapeName is a globally unique identifier for the type of a shape.
type ShapeName string

// Shape represents an autowiring shape.
// Shapes are bits of information that an autowiring function exposes to provide information to other functions that
// depend on that resource.
// This is pretty much the same as Bazel providers. See https://docs.bazel.build/versions/master/skylark/rules.html#providers
//
// Shapes in JSON look like this:
// {
//    "name": "voyager.atl-paas.net/MyShape",
//    "data": {
//      "field1": 42,
//      "field2": { "a": 7, "b": "x" }
//    }
// }
type Shape interface {
	// Name returns the name of the shape.
	Name() ShapeName
	// DeepCopyShape returns a deep copy of the shape.
	DeepCopyShape() Shape
}

// UnstructuredShape allows to unmarshal any shape from JSON/YAML into a generic representation.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type UnstructuredShape struct {
	// ShapeName is the name of the shape.
	ShapeName ShapeName `json:"name"`
	// Data is the data attached to the shape.
	// Only contains types produced by json.Unmarshal() and also int64:
	// bool, int64, float64, string, []interface{}, map[string]interface{}, json.Number and nil
	Data map[string]interface{} `json:"data,omitempty"`
}

// Name returns the name of the shape.
func (u *UnstructuredShape) Name() ShapeName {
	return u.ShapeName
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (u *UnstructuredShape) DeepCopyInto(out *UnstructuredShape) {
	*out = *u
	out.Data = runtime.DeepCopyJSON(u.Data)
}
