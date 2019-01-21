// +build !ignore_autogenerated

// Generated code
// run `make generate` to update

// Code generated by deepcopy-gen. DO NOT EDIT.

package knownshapes

import (
	wiringplugin "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASAPKey) DeepCopyInto(out *ASAPKey) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASAPKey.
func (in *ASAPKey) DeepCopy() *ASAPKey {
	if in == nil {
		return nil
	}
	out := new(ASAPKey)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *ASAPKey) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BindableEnvironmentVariables) DeepCopyInto(out *BindableEnvironmentVariables) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	in.Data.DeepCopyInto(&out.Data)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BindableEnvironmentVariables.
func (in *BindableEnvironmentVariables) DeepCopy() *BindableEnvironmentVariables {
	if in == nil {
		return nil
	}
	out := new(BindableEnvironmentVariables)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *BindableEnvironmentVariables) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BindableEnvironmentVariablesData) DeepCopyInto(out *BindableEnvironmentVariablesData) {
	*out = *in
	in.BindableShapeStruct.DeepCopyInto(&out.BindableShapeStruct)
	if in.Vars != nil {
		in, out := &in.Vars, &out.Vars
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BindableEnvironmentVariablesData.
func (in *BindableEnvironmentVariablesData) DeepCopy() *BindableEnvironmentVariablesData {
	if in == nil {
		return nil
	}
	out := new(BindableEnvironmentVariablesData)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BindableIamAccessible) DeepCopyInto(out *BindableIamAccessible) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	in.Data.DeepCopyInto(&out.Data)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BindableIamAccessible.
func (in *BindableIamAccessible) DeepCopy() *BindableIamAccessible {
	if in == nil {
		return nil
	}
	out := new(BindableIamAccessible)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *BindableIamAccessible) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BindableIamAccessibleData) DeepCopyInto(out *BindableIamAccessibleData) {
	*out = *in
	in.BindableShapeStruct.DeepCopyInto(&out.BindableShapeStruct)
	in.IAMPolicySnippet.DeepCopyInto(&out.IAMPolicySnippet)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BindableIamAccessibleData.
func (in *BindableIamAccessibleData) DeepCopy() *BindableIamAccessibleData {
	if in == nil {
		return nil
	}
	out := new(BindableIamAccessibleData)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressEndpoint) DeepCopyInto(out *IngressEndpoint) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	in.Data.DeepCopyInto(&out.Data)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressEndpoint.
func (in *IngressEndpoint) DeepCopy() *IngressEndpoint {
	if in == nil {
		return nil
	}
	out := new(IngressEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *IngressEndpoint) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressEndpointData) DeepCopyInto(out *IngressEndpointData) {
	*out = *in
	in.IngressEndpoint.DeepCopyInto(&out.IngressEndpoint)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressEndpointData.
func (in *IngressEndpointData) DeepCopy() *IngressEndpointData {
	if in == nil {
		return nil
	}
	out := new(IngressEndpointData)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RDS) DeepCopyInto(out *RDS) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RDS.
func (in *RDS) DeepCopy() *RDS {
	if in == nil {
		return nil
	}
	out := new(RDS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *RDS) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SetOfPodsSelectableByLabels) DeepCopyInto(out *SetOfPodsSelectableByLabels) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	in.Data.DeepCopyInto(&out.Data)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SetOfPodsSelectableByLabels.
func (in *SetOfPodsSelectableByLabels) DeepCopy() *SetOfPodsSelectableByLabels {
	if in == nil {
		return nil
	}
	out := new(SetOfPodsSelectableByLabels)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *SetOfPodsSelectableByLabels) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SetOfPodsSelectableByLabelsData) DeepCopyInto(out *SetOfPodsSelectableByLabelsData) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SetOfPodsSelectableByLabelsData.
func (in *SetOfPodsSelectableByLabelsData) DeepCopy() *SetOfPodsSelectableByLabelsData {
	if in == nil {
		return nil
	}
	out := new(SetOfPodsSelectableByLabelsData)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SnsSubscribable) DeepCopyInto(out *SnsSubscribable) {
	*out = *in
	out.ShapeMeta = in.ShapeMeta
	in.Data.DeepCopyInto(&out.Data)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SnsSubscribable.
func (in *SnsSubscribable) DeepCopy() *SnsSubscribable {
	if in == nil {
		return nil
	}
	out := new(SnsSubscribable)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyShape is an autogenerated deepcopy function, copying the receiver, creating a new wiringplugin.Shape.
func (in *SnsSubscribable) DeepCopyShape() wiringplugin.Shape {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SnsSubscribableData) DeepCopyInto(out *SnsSubscribableData) {
	*out = *in
	in.BindableShapeStruct.DeepCopyInto(&out.BindableShapeStruct)
	in.TopicARN.DeepCopyInto(&out.TopicARN)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SnsSubscribableData.
func (in *SnsSubscribableData) DeepCopy() *SnsSubscribableData {
	if in == nil {
		return nil
	}
	out := new(SnsSubscribableData)
	in.DeepCopyInto(out)
	return out
}
