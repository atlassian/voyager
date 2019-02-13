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

type ReleaseREST struct {
	Logger *zap.Logger
	Handler *trebuchet.ReleaseHandler
}

// NamespaceScoped returns true if the storage is namespaced
func (r *ReleaseREST) NamespaceScoped() bool {
	return true
}

// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *ReleaseREST) New() runtime.Object {
	return &trebuchet_v1.Release{}
}

// TODO: pass namespaces here
// Get finds a resource in the storage by name and returns it.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
func (r *ReleaseREST) Get(ctx context.Context, uuid string, options *metav1.GetOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	return r.Handler.GetRelease(ctx, uuid)
}

func (r *ReleaseREST) GetLatest(ctx context.Context, options *metav1.GetOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	return r.Handler.GetRelease(ctx, "namespace")
}

// Create creates a new version of a resource. If includeUninitialized is set, the object may be returned
// without completing initialization.
func (r *ReleaseREST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	release := obj.(*trebuchet_v1.Release)
	return r.Handler.CreateRelease(ctx, release)
}

