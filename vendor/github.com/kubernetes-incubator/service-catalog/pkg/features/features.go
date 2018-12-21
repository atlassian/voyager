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

package features

import (
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

const (
	// Every feature gate should add method here following this template:
	//
	// // owner: @username
	// // alpha: v1.4
	// MyFeature() bool

	// OriginatingIdentity controls whether the controller should include
	// originating identity in the header of requests
	// sent to brokers
	//
	// owner: @pmorie
	// alpha: v1.7
	OriginatingIdentity utilfeature.Feature = "OriginatingIdentity"

	// AsyncBindingOperations controls whether the controller should
	// attempt asynchronous binding operations
	//
	// owner: @mkibbe
	// alpha: v1.7
	AsyncBindingOperations utilfeature.Feature = "AsyncBindingOperations"

	// PodPreset controls whether PodPreset resource is enabled or not in the
	// API server.
	// owner: @droot
	// alpha: v0.1.6
	PodPreset utilfeature.Feature = "PodPreset"

	// NamespacedServiceBroker enables namespaced variants of ServiceBrokers,
	// ServiceClasses, and ServicePlans.
	// owner: @eriknelson & @jeremyrickard
	// alpha: v0.1.10
	NamespacedServiceBroker utilfeature.Feature = "NamespacedServiceBroker"

	// ResponseSchema enables the storage of the binding response schema
	// in ServicePlans
	// owner: @luksa
	// alpha: v0.1.12
	ResponseSchema utilfeature.Feature = "ResponseSchema"

	// UpdateDashboardURL enables the update of DashboardURL in response
	// to update service instance requests to brokers.
	// owner: @jberkhahn
	// alpha: v0.1.13
	UpdateDashboardURL utilfeature.Feature = "UpdateDashboardURL"

	// OriginatingIdentityLocking controls whether we lock OSB API resources
	// for updating while we are still processing the current spec.
	// owner: @nilebox
	// alpha: v0.1.14
	OriginatingIdentityLocking utilfeature.Feature = "OriginatingIdentityLocking"

	// ServicePlanDefaults enables applying default values to service instances
	// and bindings.
	// owner: @carolynvs
	// alpha: v0.1.32
	ServicePlanDefaults utilfeature.Feature = "ServicePlanDefaults"
)

func init() {
	utilfeature.DefaultFeatureGate.Add(defaultServiceCatalogFeatureGates)
}

// defaultServiceCatalogFeatureGates consists of all known service catalog specific feature keys.
// To add a new feature, define a key for it above and add it here. The features will be
// available throughout service catalog binaries.
var defaultServiceCatalogFeatureGates = map[utilfeature.Feature]utilfeature.FeatureSpec{
	PodPreset:                  {Default: false, PreRelease: utilfeature.Alpha},
	OriginatingIdentity:        {Default: true, PreRelease: utilfeature.GA},
	AsyncBindingOperations:     {Default: false, PreRelease: utilfeature.Alpha},
	NamespacedServiceBroker:    {Default: true, PreRelease: utilfeature.Alpha},
	ResponseSchema:             {Default: false, PreRelease: utilfeature.Alpha},
	UpdateDashboardURL:         {Default: false, PreRelease: utilfeature.Alpha},
	OriginatingIdentityLocking: {Default: true, PreRelease: utilfeature.Alpha},
	ServicePlanDefaults:        {Default: false, PreRelease: utilfeature.Alpha},
}
