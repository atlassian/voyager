package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &LocationDescriptorList{}
var _ meta_v1.ListMetaAccessor = &LocationDescriptorList{}

var _ runtime.Object = &LocationDescriptor{}
var _ meta_v1.ObjectMetaAccessor = &LocationDescriptor{}
