// Package wiringplugin provides the wiring-related types surrounding "WiringPlugin"
package wiringplugin

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WiringResultType string
type StatusResultType string

const (
	WiringResultSuccessType WiringResultType = "success"
	WiringResultFailureType WiringResultType = "failure"

	StatusResultSuccessType StatusResultType = "success"
	StatusResultFailureType StatusResultType = "failure"
)

// WiringPlugin represents an autowiring plugin.
// Autowiring plugin is an in-code representation of an autowiring function.
type WiringPlugin interface {
	// WireUp wires up the resource.
	// Error may be retriable if its an RPC error (like network error). Most errors are not retriable because
	// this method should be pure/deterministic so if it fails, it fails.
	WireUp(resource *orch_v1.StateResource, context *WiringContext) (result WiringResult)
	Status(resource *orch_v1.StateResource, context *StatusContext) (result StatusResult)
}

// WiringContext contains context information that is passed to an autowiring function to perform autowiring
// for a resource.
type WiringContext struct {
	StateMeta    meta_v1.ObjectMeta
	StateContext StateContext
	Dependencies []WiredDependency
	Dependants   []DependantResource
}

// TheOnlyDependency will return a single dependency, returning an error if there is more or less than one
func (c *WiringContext) TheOnlyDependency() (*WiredDependency, error) {
	switch len(c.Dependencies) {
	case 0:
		return nil, errors.New("must depend on a single resource, but none was found")
	case 1:
		return &c.Dependencies[0], nil
	default:
		return nil, errors.Errorf("must depend on a single resource, but multiple were found")
	}
}

// FindTheOnlyDependency will return a single dependency if found, returning and error if more than one is found
func (c *WiringContext) FindTheOnlyDependency() (*WiredDependency, bool /* found */, error) {
	switch len(c.Dependencies) {
	case 0:
		return nil, false, nil
	case 1:
		return &c.Dependencies[0], true, nil
	default:
		return nil, false, errors.Errorf("can only depend on a single resource, but multiple were found")
	}
}

// WiredDependency represents a resource that has been processed by a corresponding autowiring function.
type WiredDependency struct {
	Name     voyager.ResourceName
	Contract ResourceContract
	// Attributes are attributes attached to the edge between resources.
	Attributes map[string]interface{}
}

// DependantResource represents a resource that depends on the resource that is currently being processed.
type DependantResource struct {
	Name voyager.ResourceName
	Type voyager.ResourceType
	// Attributes are attributes attached to the edge between resources.
	Attributes map[string]interface{}
	Resource   orch_v1.StateResource
}

// ResourceContract contains information about a resource for consumption by other autowiring functions.
// It is the API of a resource that can be depended upon and hence should not change unexpectedly without
// a proper migration path to a new version.
type ResourceContract struct {
	Shapes []Shape `json:"shapes,omitempty"`
}

type WiringResult interface {
	StatusType() WiringResultType
}

type WiringResultSuccess struct {
	Contract  ResourceContract
	Resources []smith_v1.Resource
}

func (w *WiringResultSuccess) StatusType() WiringResultType {
	return WiringResultSuccessType
}

type WiringResultFailure struct {
	Error            error
	IsExternalError  bool
	IsRetriableError bool
}

func (w *WiringResultFailure) StatusType() WiringResultType {
	return WiringResultFailureType
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

	ServiceName voyager.ServiceName

	// ServiceProperties is extra metadata we pulled from the EntanglerContext
	// which comes from a ConfigMap tied to the State.
	ServiceProperties orch_meta.ServiceProperties

	// Tags is the final computed tags that include business_unit and service_name
	// and etc.
	Tags map[voyager.Tag]string

	// ClusterConfig is the cluster config.
	ClusterConfig ClusterConfig
}

type ClusterConfig struct {
	// ClusterDomainName is the domain name of the ingress.
	ClusterDomainName string
	KittClusterEnv    string
	Kube2iamAccount   string
}

type BundleResource struct {
	// Resource is the Smith resource that has been produced as the result of processing an Orchestration StateResource.
	Resource smith_v1.Resource `json:"resource"`
	// Status is the status of that object as reported by Smith.
	Status smith_v1.ResourceStatusData `json:"status"`
}

type StatusContext struct {
	// BundleResources is a list of resources and their statuses in a Bundle.
	// Only resources for a particular StateResource are in the list.
	BundleResources []BundleResource `json:"bundleResources,omitempty"`
	// PluginStatuses is a list of statuses for Smith plugins used in a Bundle.
	PluginStatuses []smith_v1.PluginStatus `json:"pluginStatuses,omitempty"`
}

type StatusResult interface {
	StatusType() StatusResultType
}

type StatusResultSuccess struct {
	ResourceStatusData orch_v1.ResourceStatusData `json:"resourceStatusData"`
}

func (s *StatusResultSuccess) StatusType() StatusResultType {
	return StatusResultSuccessType
}

type StatusResultFailure struct {
	Error           error
	IsExternalError bool
}

func (s *StatusResultFailure) StatusType() StatusResultType {
	return StatusResultFailureType
}
