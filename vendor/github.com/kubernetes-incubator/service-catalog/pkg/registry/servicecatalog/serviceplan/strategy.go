/*
Copyright 2018 The Kubernetes Authors.

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

package serviceplan

// this was copied from where else and edited to fit our objects

import (
	"context"

	"github.com/kubernetes-incubator/service-catalog/pkg/api"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage/names"

	"github.com/golang/glog"
	sc "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	scv "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/validation"
)

// NewScopeStrategy returns a new NamespaceScopedStrategy for service planes
func NewScopeStrategy() rest.NamespaceScopedStrategy {
	return servicePlanRESTStrategies
}

// implements interfaces RESTCreateStrategy, RESTUpdateStrategy, RESTDeleteStrategy,
// NamespaceScopedStrategy
type servicePlanRESTStrategy struct {
	runtime.ObjectTyper // inherit ObjectKinds method
	names.NameGenerator // GenerateName method for CreateStrategy
}

// implements interface RESTUpdateStrategy. This implementation validates updates to
// servicePlan.Status updates only and disallows any modifications to the ServicePlan.Spec.
type servicePlanStatusRESTStrategy struct {
	servicePlanRESTStrategy
}

var (
	servicePlanRESTStrategies = servicePlanRESTStrategy{
		// embeds to pull in existing code behavior from upstream

		ObjectTyper: api.Scheme,
		// use the generator from upstream k8s, or implement method
		// `GenerateName(base string) string`
		NameGenerator: names.SimpleNameGenerator,
	}
	_ rest.RESTCreateStrategy = servicePlanRESTStrategies
	_ rest.RESTUpdateStrategy = servicePlanRESTStrategies
	_ rest.RESTDeleteStrategy = servicePlanRESTStrategies

	servicePlanStatusUpdateStrategy = servicePlanStatusRESTStrategy{
		servicePlanRESTStrategies,
	}
	_ rest.RESTUpdateStrategy = servicePlanStatusUpdateStrategy
)

// Canonicalize does not transform a ServicePlan.
func (servicePlanRESTStrategy) Canonicalize(obj runtime.Object) {
	_, ok := obj.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to create")
	}
}

// NamespaceScoped returns false as ServicePlan are not scoped to a namespace.
func (servicePlanRESTStrategy) NamespaceScoped() bool {
	return true
}

// PrepareForCreate receives the incoming ServicePlan.
func (servicePlanRESTStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	_, ok := obj.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to create")
	}
	// service plan is a data record and has no status to track
}

func (servicePlanRESTStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return scv.ValidateServicePlan(obj.(*sc.ServicePlan))
}

func (servicePlanRESTStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (servicePlanRESTStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (servicePlanRESTStrategy) PrepareForUpdate(ctx context.Context, new, old runtime.Object) {
	newServicePlan, ok := new.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to update to")
	}
	oldServicePlan, ok := old.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to update from")
	}

	newServicePlan.Spec.ServiceClassRef = oldServicePlan.Spec.ServiceClassRef
	newServicePlan.Spec.ServiceBrokerName = oldServicePlan.Spec.ServiceBrokerName
}

func (servicePlanRESTStrategy) ValidateUpdate(ctx context.Context, new, old runtime.Object) field.ErrorList {
	newServicePlan, ok := new.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to validate to")
	}
	oldServicePlan, ok := old.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to validate from")
	}

	return scv.ValidateServicePlanUpdate(newServicePlan, oldServicePlan)
}

func (servicePlanStatusRESTStrategy) PrepareForUpdate(ctx context.Context, new, old runtime.Object) {
	newServiceClass, ok := new.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to update to")
	}
	oldServiceClass, ok := old.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to update from")
	}
	// Status changes are not allowed to update spec
	newServiceClass.Spec = oldServiceClass.Spec
}

func (servicePlanStatusRESTStrategy) ValidateUpdate(ctx context.Context, new, old runtime.Object) field.ErrorList {
	newServicePlan, ok := new.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to validate to")
	}
	oldServicePlan, ok := old.(*sc.ServicePlan)
	if !ok {
		glog.Fatal("received a non-ServicePlan object to validate from")
	}

	return scv.ValidateServicePlanUpdate(newServicePlan, oldServicePlan)
}
