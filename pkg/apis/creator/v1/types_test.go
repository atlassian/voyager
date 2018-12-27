package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &ServiceList{}
var _ meta_v1.ListMetaAccessor = &ServiceList{}

var _ runtime.Object = &Service{}
var _ meta_v1.ObjectMetaAccessor = &Service{}
