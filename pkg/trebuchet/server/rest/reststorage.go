package rest

import (
	"context"

	trebuchet_v1 "github.com/atlassian/voyager/pkg/apis/trebuchet/v1"
	"go.uber.org/zap"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

type REST struct {
	Logger *zap.Logger
}

// NamespaceScoped returns true if the storage is namespaced
func (r *REST) NamespaceScoped() bool {
	return false
}

// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *REST) New() runtime.Object {
	return &trebuchet_v1.Release{}
}

// NewList returns an empty object that can be used with the List call.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object)
func (r *REST) NewList() runtime.Object {
	return &trebuchet_v1.ReleaseList{}
}

// TODO: fill in following methods with actual code

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *REST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	return nil, nil
}

// Get finds a resource in the storage by name and returns it.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
func (r *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return nil, nil
}

// Create creates a new version of a resource. If includeUninitialized is set, the object may be returned
// without completing initialization.
func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	return nil, nil
}

// Update finds a resource in the storage and updates it. Some implementations
// may allow updates creates the object - they should set the created boolean
// to true.
func (r *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return nil, false, nil
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
	return nil, false, nil
}
