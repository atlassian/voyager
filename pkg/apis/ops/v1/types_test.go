package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &RouteList{}
var _ meta_v1.ListMetaAccessor = &RouteList{}

var _ runtime.Object = &Route{}
var _ meta_v1.ObjectMetaAccessor = &Route{}
