package rest

import (
	"context"

	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/creator"
	"github.com/atlassian/voyager/pkg/util/logz"
	"go.uber.org/zap"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type REST struct {
	Logger  *zap.Logger
	Handler *creator.ServiceHandler
}

// NamespaceScoped returns true if the storage is namespaced
func (r *REST) NamespaceScoped() bool {
	return false
}

// New returns an empty object that can be used with Create and Update after request data has been put into it.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *REST) New() runtime.Object {
	return &creator_v1.Service{}
}

// NewList returns an empty object that can be used with the List call.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *REST) NewList() runtime.Object {
	return &creator_v1.ServiceList{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *REST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	return r.Handler.ServiceListNew(ctx)
}

// Get finds a resource in the storage by name and returns it.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
func (r *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	return r.Handler.ServiceGet(ctx, name)
}

// Create creates a new version of a resource. If includeUninitialized is set, the object may be returned
// without completing initialization.
func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	service := obj.(*creator_v1.Service)
	return r.Handler.ServiceCreate(ctx, service)
}

// Update finds a resource in the storage and updates it. Some implementations
// may allow updates creates the object - they should set the created boolean
// to true.
func (r *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	service, err := r.Handler.ServiceUpdate(ctx, name, objInfo)
	return service, false, err
}

// Delete finds a resource in the storage and deletes it.
// If options are provided, the resource will attempt to honor them or return an invalid
// request error.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
// Delete *may* return the object that was deleted, or a status object indicating additional
// information about deletion.
// It also returns a boolean which is set to true if the resource was instantly
// deleted or false if it will be deleted asynchronously.
func (r *REST) Delete(ctx context.Context, name string, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	ctx = logz.CreateContextWithLogger(ctx, r.Logger)
	service, err := r.Handler.ServiceDelete(ctx, name)
	return service, err == nil, err
}
