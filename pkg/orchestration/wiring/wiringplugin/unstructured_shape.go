package wiringplugin

import "k8s.io/apimachinery/pkg/runtime"

// UnstructuredShape allows to unmarshal any shape from JSON/YAML into a generic representation.
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type UnstructuredShape struct {
	ShapeMeta `json:",inline"`
	// Data is the data attached to the shape.
	// Only contains types produced by json.Unmarshal() and also int64:
	// bool, int64, float64, string, []interface{}, map[string]interface{}, json.Number and nil
	Data map[string]interface{} `json:"data,omitempty"`
}

// DeepCopyInto handle the interface{} deepcopy (which k8s can't autogen,
// since it doesn't know it's JSON).
func (u *UnstructuredShape) DeepCopyInto(out *UnstructuredShape) {
	*out = *u
	out.Data = runtime.DeepCopyJSON(u.Data)
}
