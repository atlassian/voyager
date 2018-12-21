/*
Copyright 2017 The Kubernetes Authors.

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

package clusterserviceplan

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
	errNotAClusterServicePlan = errors.New("not a ClusterServicePlan")
)

// NewSingular returns a new shell of a ClusterServicePlan, according to the given namespace and
// name
func NewSingular(ns, name string) runtime.Object {
	return &servicecatalog.ClusterServicePlan{
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterServicePlan",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

// EmptyObject returns an empty ClusterServicePlan
func EmptyObject() runtime.Object {
	return &servicecatalog.ClusterServicePlan{}
}

// NewList returns a new shell of a ClusterServicePlan list
func NewList() runtime.Object {
	return &servicecatalog.ClusterServicePlanList{
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterServicePlanList",
		},
		Items: []servicecatalog.ClusterServicePlan{},
	}
}

// CheckObject returns a non-nil error if obj is not a ClusterServicePlan object
func CheckObject(obj runtime.Object) error {
	_, ok := obj.(*servicecatalog.ClusterServicePlan)
	if !ok {
		return errNotAClusterServicePlan
	}
	return nil
}

// Match determines whether an Instance matches a field and label
// selector.
func Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// toSelectableFields returns a field set that represents the object for matching purposes.
func toSelectableFields(servicePlan *servicecatalog.ClusterServicePlan) fields.Set {
	// The purpose of allocation with a given number of elements is to reduce
	// amount of allocations needed to create the fields.Set. If you add any
	// field here or the number of object-meta related fields changes, this should
	// be adjusted.
	// You also need to modify
	// pkg/apis/servicecatalog/v1beta1/conversion[_test].go
	spSpecificFieldsSet := make(fields.Set, 4)
	spSpecificFieldsSet["spec.clusterServiceBrokerName"] = servicePlan.Spec.ClusterServiceBrokerName
	spSpecificFieldsSet["spec.clusterServiceClassRef.name"] = servicePlan.Spec.ClusterServiceClassRef.Name
	spSpecificFieldsSet["spec.externalName"] = servicePlan.Spec.ExternalName
	spSpecificFieldsSet["spec.externalID"] = servicePlan.Spec.ExternalID
	return generic.AddObjectMetaFieldsSet(spSpecificFieldsSet, &servicePlan.ObjectMeta, false)
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, bool, error) {
	servicePlan, ok := obj.(*servicecatalog.ClusterServicePlan)
	if !ok {
		return nil, nil, false, fmt.Errorf("given object is not a ClusterServicePlan")
	}
	return labels.Set(servicePlan.ObjectMeta.Labels), toSelectableFields(servicePlan), servicePlan.Initializers != nil, nil
}

// NewStorage creates a new rest.Storage responsible for accessing
// ClusterServicePlan resources
func NewStorage(opts server.Options) (rest.Storage, rest.Storage) {
	prefix := "/" + opts.ResourcePrefix()

	storageInterface, dFunc := opts.GetStorage(
		&servicecatalog.ClusterServicePlan{},
		prefix,
		clusterServicePlanRESTStrategies,
		NewList,
		nil,
		storage.NoTriggerPublisher,
	)

	store := registry.Store{
		NewFunc:     EmptyObject,
		NewListFunc: NewList,
		KeyRootFunc: opts.KeyRootFunc(),
		KeyFunc:     opts.KeyFunc(false),
		// Retrieve the name field of the resource.
		ObjectNameFunc: func(obj runtime.Object) (string, error) {
			return scmeta.GetAccessor().Name(obj)
		},
		// Used to match objects based on labels/fields for list.
		PredicateFunc: Match,
		// DefaultQualifiedResource should always be plural
		DefaultQualifiedResource: servicecatalog.Resource("clusterserviceplans"),

		CreateStrategy: clusterServicePlanRESTStrategies,
		UpdateStrategy: clusterServicePlanRESTStrategies,
		DeleteStrategy: clusterServicePlanRESTStrategies,

		TableConvertor: tableconvertor.NewTableConvertor(
			[]metav1beta1.TableColumnDefinition{
				{Name: "Name", Type: "string", Format: "name"},
				{Name: "External-Name", Type: "string"},
				{Name: "Broker", Type: "string"},
				{Name: "Class", Type: "string"},
				{Name: "Age", Type: "string"},
			},
			func(obj runtime.Object, m metav1.Object, name, age string) ([]interface{}, error) {
				plan := obj.(*servicecatalog.ClusterServicePlan)
				cells := []interface{}{
					name,
					plan.Spec.ExternalName,
					plan.Spec.ClusterServiceBrokerName,
					plan.Spec.ClusterServiceClassRef.Name,
					age,
				}
				return cells, nil
			},
		),

		Storage:     storageInterface,
		DestroyFunc: dFunc,
	}

	statusStore := store
	statusStore.UpdateStrategy = clusterServicePlanStatusUpdateStrategy

	return &store, &StatusREST{&statusStore}
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

// New returns a new ClusterServicePlan
func (r *StatusREST) New() runtime.Object {
	return &servicecatalog.ClusterServicePlan{}
}

// Get retrieves the object from the storage. It is required to support Patch
// and to implement the rest.Getter interface.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an object and it
// implements rest.Updater interface
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
}
