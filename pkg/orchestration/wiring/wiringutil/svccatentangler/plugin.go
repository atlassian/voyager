package svccatentangler

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ShapesFunc is used to return a list of shapes, it is used in the SvcCatEntangler as a way to return a list
// of shapes for the resource to be used as input to the wiring functions of the dependants.
//
// The `resource` is the orchestration level resource that was transformed into `smithResource`.
type ShapesFunc func(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* external */, bool /* retriable */, error)

// SvcCatEntagler aims to abstract some of the work involved in writing a WiringPlugin functions. It assumes that every
// WiringPlugin will provide a bundle spec through InstanceSpec(), the metadata for that spec through ObjectMeta() and a
// list of smith references through References().
//
// This is for WiringPlugins that will create a ServiceInstance.

// DEPRECATED: Use helper functions from 'osb' package instead.
type SvcCatEntangler struct {

	// This identifies what resource types can be processed.
	ResourceType voyager.ResourceType

	// These are the OSB broker class and plan identifiers
	ClusterServiceClassExternalID servicecatalog.ClassExternalID
	ClusterServicePlanExternalID  servicecatalog.PlanExternalID

	// InstanceSpec will return the JSON marshaled form of the bundle resource's spec. this
	// service instance, service binding, deployment, ingress, or other resource.
	// In the case of an error, the error is returned as well as a booleans retriable and external
	// for if the error is retriable and if the error represents an external error (or user error).
	InstanceSpec func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]byte, bool /* externalError */, bool /* retriable */, error)

	// ObjectMeta will return the ObjectMeta for the ServiceInstance.
	// If nil, it's skipped.
	// In the case of an error, the error is returned as well as a booleans retriable and external
	// for if the error is retriable and if the error represents an external error (or user error).
	ObjectMeta func(*orch_v1.StateResource, *wiringplugin.WiringContext) (meta_v1.ObjectMeta, bool /* externalError */, bool /* retriable */, error)

	// References will return a list of Smith references used by the any part of the resulting Smith resource. A Smith
	// reference looks something like "!{refname}" and require a smith reference entry with the same name to work.
	// If nil, it's skipped.
	// In the case of an error, the error is returned as well as a booleans retriable and external
	// for if the error is retriable and if the error represents an external error (or user error).
	References func(*orch_v1.StateResource, *wiringplugin.WiringContext) ([]smith_v1.Reference, bool /* externalError */, bool /* retriable */, error)

	// See documentation for ShapesFunc
	// If nil, it's skipped.
	Shapes ShapesFunc
}

type partialSpec struct {
	InstanceID string `json:"instanceId"`
}

// Gets instanceId from resource's spec if present or "" otherwise
// If the spec is empty or does not contain instance ID, then this returns the empty string.
func InstanceID(resourceSpec *runtime.RawExtension) (string, error) {
	if resourceSpec == nil {
		return "", nil
	}

	spec := partialSpec{}
	err := json.Unmarshal(resourceSpec.Raw, &spec)
	if err != nil {
		return "", errors.Wrap(err, "unmarshalling StateResource failed")
	}

	return spec.InstanceID, nil
}

func (e *SvcCatEntangler) constructServiceInstance(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (smith_v1.Resource, bool /* externalError */, bool /* retriable */, error) {
	serviceInstanceExternalID, err := InstanceID(resource.Spec)
	if err != nil {
		return smith_v1.Resource{}, false, false, err
	}
	serviceInstanceSpecBytes, external, retriable, err := e.InstanceSpec(resource, context)
	if err != nil {
		return smith_v1.Resource{}, external, retriable, err
	}
	var parameters *runtime.RawExtension
	if serviceInstanceSpecBytes != nil {
		parameters = &runtime.RawExtension{
			Raw: serviceInstanceSpecBytes,
		}
	}

	var objectMeta meta_v1.ObjectMeta
	if e.ObjectMeta != nil {
		objectMeta, external, retriable, err = e.ObjectMeta(resource, context)
		if err != nil {
			return smith_v1.Resource{}, external, retriable, err
		}
	}

	if objectMeta.Name == "" {
		objectMeta.SetName(wiringutil.ServiceInstanceMetaName(resource.Name))
	}

	var references []smith_v1.Reference
	if e.References != nil {
		references, external, retriable, err = e.References(resource, context)
		if err != nil {
			return smith_v1.Resource{}, external, retriable, err
		}
	}

	return smith_v1.Resource{
		Name:       wiringutil.ServiceInstanceResourceName(resource.Name),
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: objectMeta,
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalID: string(e.ClusterServiceClassExternalID),
						ClusterServicePlanExternalID:  string(e.ClusterServicePlanExternalID),
					},
					Parameters: parameters,
					ExternalID: serviceInstanceExternalID,
				},
			},
		},
	}, false, false, nil
}

func (e *SvcCatEntangler) constructResourceContract(resource *orch_v1.StateResource, smithResource *smith_v1.Resource, context *wiringplugin.WiringContext) (*wiringplugin.ResourceContract, bool /*externalError*/, bool /*retriable*/, error) {
	supportedShapes := []wiringplugin.Shape{}

	if e.Shapes != nil {
		shapes, external, retriable, err := e.Shapes(resource, smithResource, context)
		if err != nil {
			return nil, external, retriable, errors.Wrapf(err, "failed to compute shapes for resource %q of type %q", resource.Name, resource.Type)
		}
		supportedShapes = append(supportedShapes, shapes...)
	}
	return &wiringplugin.ResourceContract{
		Shapes: supportedShapes,
	}, false, false, nil
}

func (e *SvcCatEntangler) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != e.ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, external, retriable, err := e.constructServiceInstance(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	resourceContract, external, retriable, err := e.constructResourceContract(resource, &serviceInstance, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	return &wiringplugin.WiringResultSuccess{
		Contract:  *resourceContract,
		Resources: []smith_v1.Resource{serviceInstance},
	}
}
