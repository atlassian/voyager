package rest

import (
	"context"
	"github.com/atlassian/voyager/pkg/util/logz"

	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"github.com/atlassian/voyager/pkg/trebuchet"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type ReleaseGroupREST struct {
	Logger *zap.Logger
	Handler *trebuchet.ReleaseGroupHandler
}

// NamespaceScoped returns true if the storage is namespaced
func (r *ReleaseGroupREST) NamespaceScoped() bool {
	return true
}

// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *ReleaseGroupREST) New() runtime.Object {
	return &trebuchet_v1.Release{}
}

// NewList returns an empty object that can be used with the List call.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *ReleaseGroupREST) NewList() runtime.Object {
	return &trebuchet_v1.ReleaseList{}
}

// TODO: fill in following methods with actual code

// Get finds a resource in the storage by name and returns it.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
func (r *ReleaseGroupREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return nil, nil
}

// Create creates a new version of a resource. If includeUninitialized is set, the object may be returned
// without completing initialization.
func (r *ReleaseGroupREST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	releaseGroup := obj.(*trebuchet_v1.ReleaseGroup)
	return r.Handler.CreateOrUpdateReleaseGroup(ctx, releaseGroup)
}

func (r *ReleaseGroupREST) Delete(ctx context.Context, obj runtime.Object, options *metav1.DeleteOptions) (error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	releaseGroup := obj.(*trebuchet_v1.ReleaseGroup)
	return r.Handler.DeleteReleaseGroup(ctx, releaseGroup)
}
