package rest

import (
	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
)

type REST struct {
	Logger *zap.Logger
}

// NamespaceScoped returns true if the storage is namespaced
func (r *REST) NamespaceScoped() bool {
	return false
}

// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *REST) NewRelease() runtime.Object {
	return &trebuchet_v1.Release{}
}
