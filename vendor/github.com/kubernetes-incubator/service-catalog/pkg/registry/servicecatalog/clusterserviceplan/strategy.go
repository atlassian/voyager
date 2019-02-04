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

package clusterserviceplan

// this was copied from where else and edited to fit our objects

import (
	"context"

	"github.com/kubernetes-incubator/service-catalog/pkg/api"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage/names"

	sc "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	scv "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/validation"
	"k8s.io/klog"
)

// NewScopeStrategy returns a new NamespaceScopedStrategy for service planes
func NewScopeStrategy() rest.NamespaceScopedStrategy {
	return clusterServicePlanRESTStrategies
}

// implements interfaces RESTCreateStrategy, RESTUpdateStrategy, RESTDeleteStrategy,
// NamespaceScopedStrategy
type clusterServicePlanRESTStrategy struct {
	runtime.ObjectTyper // inherit ObjectKinds method
	names.NameGenerator // GenerateName method for CreateStrategy
}

// implements interface RESTUpdateStrategy. This implementation validates updates to
// servicePlan.Status updates only and disallows any modifications to the ClusterServicePlan.Spec.
type clusterServicePlanStatusRESTStrategy struct {
	clusterServicePlanRESTStrategy
}

var (
	clusterServicePlanRESTStrategies = clusterServicePlanRESTStrategy{
		// embeds to pull in existing code behavior from upstream

		ObjectTyper: api.Scheme,
		// use the generator from upstream k8s, or implement method
		// `GenerateName(base string) string`
		NameGenerator: names.SimpleNameGenerator,
	}
	_ rest.RESTCreateStrategy = clusterServicePlanRESTStrategies
	_ rest.RESTUpdateStrategy = clusterServicePlanRESTStrategies
	_ rest.RESTDeleteStrategy = clusterServicePlanRESTStrategies

	clusterServicePlanStatusUpdateStrategy = clusterServicePlanStatusRESTStrategy{
		clusterServicePlanRESTStrategies,
	}
	_ rest.RESTUpdateStrategy = clusterServicePlanStatusUpdateStrategy
)

// Canonicalize does not transform a ClusterServicePlan.
func (clusterServicePlanRESTStrategy) Canonicalize(obj runtime.Object) {
	_, ok := obj.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to create")
	}
}

// NamespaceScoped returns false as ClusterServicePlan are not scoped to a namespace.
func (clusterServicePlanRESTStrategy) NamespaceScoped() bool {
	return false
}

// PrepareForCreate receives the incoming ClusterServicePlan.
func (clusterServicePlanRESTStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	_, ok := obj.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to create")
	}
	// service plan is a data record and has no status to track
}

func (clusterServicePlanRESTStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return scv.ValidateClusterServicePlan(obj.(*sc.ClusterServicePlan))
}

func (clusterServicePlanRESTStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (clusterServicePlanRESTStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (clusterServicePlanRESTStrategy) PrepareForUpdate(ctx context.Context, new, old runtime.Object) {
	newServicePlan, ok := new.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to update to")
	}
	oldServicePlan, ok := old.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to update from")
	}

	newServicePlan.Spec.ClusterServiceClassRef = oldServicePlan.Spec.ClusterServiceClassRef
	newServicePlan.Spec.ClusterServiceBrokerName = oldServicePlan.Spec.ClusterServiceBrokerName
}

func (clusterServicePlanRESTStrategy) ValidateUpdate(ctx context.Context, new, old runtime.Object) field.ErrorList {
	newServicePlan, ok := new.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to validate to")
	}
	oldServicePlan, ok := old.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to validate from")
	}

	return scv.ValidateClusterServicePlanUpdate(newServicePlan, oldServicePlan)
}

func (clusterServicePlanStatusRESTStrategy) PrepareForUpdate(ctx context.Context, new, old runtime.Object) {
	newServiceClass, ok := new.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to update to")
	}
	oldServiceClass, ok := old.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to update from")
	}
	// Status changes are not allowed to update spec
	newServiceClass.Spec = oldServiceClass.Spec
}

func (clusterServicePlanStatusRESTStrategy) ValidateUpdate(ctx context.Context, new, old runtime.Object) field.ErrorList {
	newServicePlan, ok := new.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to validate to")
	}
	oldServicePlan, ok := old.(*sc.ClusterServicePlan)
	if !ok {
		klog.Fatal("received a non-ClusterServicePlan object to validate from")
	}

	return scv.ValidateClusterServicePlanUpdate(newServicePlan, oldServicePlan)
}
