// Package wiringplugin provides the wiring-related types surrounding "WiringPlugin"
package wiringplugin

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WiringPlugin interface {
	// WireUp wires up the resource.
	// Error may be retriable if its an RPC error (like network error). Most errors are not retriable because
	// this method should be pure/deterministic so if it fails, it fails.
	WireUp(resource *orch_v1.StateResource, context *WiringContext) (*WiringResult, bool /*retriable*/, error)
}

type WiringContext struct {
	StateMeta    meta_v1.ObjectMeta
	StateContext StateContext
	Dependencies []WiredDependency
	Dependents   []orch_v1.StateResource
}

type WiredDependency struct {
	Name           voyager.ResourceName
	Type           voyager.ResourceType
	SmithResources []smith_v1.Resource
	Attributes     map[string]interface{}
}

type WiringResult struct {
	Resources []WiredSmithResource
}

type WiredSmithResource struct {
	SmithResource smith_v1.Resource
	Exposed       bool
}

// StateContext is used as input for the plugins. Everything in the StateContext
// is constructed from a combination of the Entangler struct, the State resource,
// and the EntanglerContext.
// This has a few legacy concepts tied to Atlassian which we could probably move
// to being read from user-provided autowiring functions.
type StateContext struct {
	// Location is constructed from a combination of ClusterLocation and the label
	// from the EntanglerContext.
	Location voyager.Location

	// LegacyConfig is read by a function specified in the entangler struct.
	// TODO this is a temporary container for 'stuff that's in Micros config.js'.
	// It needs to be migrated ... somewhere. Either to the providers, the cluster
	// config, a configuration file, ...
	LegacyConfig legacy.Config

	// This is a legacy concept that we should at least be consistent with.
	// ServiceName is not a voyager concept and will need to be passed to
	// micros type OSB resources, and we may not always use the State resource
	// name as the ServiceName.
	ServiceName string

	// ServiceProperties is extra metadata we pulled from the EntanglerContext
	// which comes from a ConfigMap tied to the State.
	ServiceProperties orch_meta.ServiceProperties

	// Tags is the final computed tags that include business_unit and service_name
	// and etc.
	Tags map[voyager.Tag]string

	// ClusterConfig is the cluster config.
	ClusterConfig orchestration.ClusterConfig
}
