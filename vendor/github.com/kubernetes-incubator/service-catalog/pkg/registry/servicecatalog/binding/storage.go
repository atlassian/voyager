/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package binding

import (
	"context"
	"errors"
	"fmt"

	scmeta "github.com/kubernetes-incubator/service-catalog/pkg/api/meta"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	"github.com/kubernetes-incubator/service-catalog/pkg/registry/servicecatalog/server"
	"github.com/kubernetes-incubator/service-catalog/pkg/registry/servicecatalog/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
)

var (
	errNotAServiceBinding = errors.New("not a binding")
)

// NewSingular returns a new shell of a service binding, according to the given namespace and
// name
func NewSingular(ns, name string) runtime.Object {
	return &servicecatalog.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

// EmptyObject returns an empty binding
func EmptyObject() runtime.Object {
	return &servicecatalog.ServiceBinding{}
}

// NewList returns a new shell of a binding list
func NewList() runtime.Object {
	return &servicecatalog.ServiceBindingList{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceBindingList",
		},
		Items: []servicecatalog.ServiceBinding{},
	}
}

// CheckObject returns a non-nil error if obj is not a binding object
func CheckObject(obj runtime.Object) error {
	_, ok := obj.(*servicecatalog.ServiceBinding)
	if !ok {
		return errNotAServiceBinding
	}
	return nil
}

// Match determines whether an ServiceInstance matches a field and label
// selector.
func Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// toSelectableFields returns a field set that represents the object for matching purposes.
func toSelectableFields(binding *servicecatalog.ServiceBinding) fields.Set {
	// If you add a new selectable field, you also need to modify
	// pkg/apis/servicecatalog/v1beta1/conversion[_test].go
	specFieldSet := make(fields.Set, 1)
	specFieldSet["spec.externalID"] = binding.Spec.ExternalID
	return generic.AddObjectMetaFieldsSet(specFieldSet, &binding.ObjectMeta, true)
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, bool, error) {
	binding, ok := obj.(*servicecatalog.ServiceBinding)
	if !ok {
		return nil, nil, false, fmt.Errorf("given object is not a ServiceBinding")
	}
	return labels.Set(binding.ObjectMeta.Labels), toSelectableFields(binding), binding.Initializers != nil, nil
}

// NewStorage creates a new rest.Storage responsible for accessing ServiceBinding
// resources
func NewStorage(opts server.Options) (rest.Storage, rest.Storage, error) {
	prefix := "/" + opts.ResourcePrefix()

	storageInterface, dFunc := opts.GetStorage(
		&servicecatalog.ServiceBinding{},
		prefix,
		bindingRESTStrategies,
		NewList,
		nil,
		storage.NoTriggerPublisher,
	)

	store := registry.Store{
		NewFunc: EmptyObject,
		// NewListFunc returns an object capable of storing results of an etcd list.
		NewListFunc: NewList,
		KeyRootFunc: opts.KeyRootFunc(),
		KeyFunc:     opts.KeyFunc(true),
		// Retrieve the name field of the resource.
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			return scmeta.GetAccessor().Name(obj)
		},
		// Used to match objects based on labels/fields for list.
		PredicateFunc: Match,
		// DefaultQualifiedResource should always be plural
		DefaultQualifiedResource: servicecatalog.Resource("servicebindings"),

		CreateStrategy:          bindingRESTStrategies,
		UpdateStrategy:          bindingRESTStrategies,
		DeleteStrategy:          bindingRESTStrategies,
		EnableGarbageCollection: true,

		TableConvertor: tableconvertor.NewTableConvertor(
			[]metav1beta1.TableColumnDefinition{
				{Name: "Name", Type: "string", Format: "name"},
				{Name: "Service-Instance", Type: "string"},
				{Name: "Secret-Name", Type: "string"},
				{Name: "Status", Type: "string"},
				{Name: "Age", Type: "string"},
			},
			func(obj runtime.Object, m metav1.Object, name, age string) ([]interface{}, error) {
				getStatus := func(status servicecatalog.ServiceBindingStatus) string {
					if len(status.Conditions) > 0 {
						condition := status.Conditions[len(status.Conditions)-1]
						if condition.Status == servicecatalog.ConditionTrue {
							return string(condition.Type)
						}
						return condition.Reason
					}
					return ""
				}

				binding := obj.(*servicecatalog.ServiceBinding)
				cells := []interface{}{
					name,
					binding.Spec.ServiceInstanceRef.Name,
					binding.Spec.SecretName,
					getStatus(binding.Status),
					age,
				}
				return cells, nil
			},
		),

		Storage:     storageInterface,
		DestroyFunc: dFunc,
	}

	options := &generic.StoreOptions{RESTOptions: opts.EtcdOptions.RESTOptions, AttrFunc: GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		panic(err) // TODO: Propagate error up
	}

	statusStore := store
	statusStore.UpdateStrategy = bindingStatusUpdateStrategy

	return &store, &StatusREST{&statusStore}, nil
}

// StatusREST defines the REST operations for the status subresource via
// implementation of various rest interfaces.  It supports the http verbs GET,
// PATCH, and PUT.
type StatusREST struct {
	store *registry.Store
}

var (
	_ rest.Storage = &StatusREST{}
	_ rest.Getter  = &StatusREST{}
	_ rest.Updater = &StatusREST{}
)

// New returns a new ServiceBinding.
func (r *StatusREST) New() runtime.Object {
	return EmptyObject()
}

// Get retrieves the object from the storage. It is required to support Patch
// and to implement the rest.Getter interface.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an object and implements the rest.Updater
// interface.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
}
