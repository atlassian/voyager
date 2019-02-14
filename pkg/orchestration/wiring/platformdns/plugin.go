package platformdns

import (
	"encoding/json"

	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	apiplatformdns "github.com/atlassian/voyager/pkg/orchestration/wiring/platformdns/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	kubeIngressRefMetadata         = "metadata"
	kubeIngressRefMetadataEndpoint = "endpoint"
)

type autowiringOnlySpec struct {
	Target          string              `json:"target"`
	ServiceName     voyager.ServiceName `json:"serviceName"`
	EnvironmentType string              `json:"environmentType"`
}

type WiringPlugin struct {
}

func New() *WiringPlugin {
	return &WiringPlugin{}
}

func (p *WiringPlugin) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != apiplatformdns.ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, apiplatformdns.ClusterServiceClassExternalID, apiplatformdns.ClusterServicePlanExternalID)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	instanceParameters, external, retriable, err := instanceParameters(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}
	serviceInstance.Spec.Parameters = &runtime.RawExtension{
		Raw: instanceParameters,
	}

	references, external, retriable, err := getReferences(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
	}

	instanceResourceName := wiringutil.ServiceInstanceResourceName(resource.Name)

	return wiringutil.SingleWiringResult(instanceResourceName, serviceInstance, nil, references)
}

func instanceParameters(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, bool /* externalErr */, bool /* retriable */, error) {
	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var ourSpec autowiringOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &ourSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}
		if ourSpec != (autowiringOnlySpec{}) {
			return nil, true, false, errors.Errorf("unsupported parameters were provided: %+v", ourSpec)
		}
	}

	references, external, retriable, err := getReferences(resource, context)
	if err != nil {
		// User error from providing invalid dependency
		return nil, external, retriable, err
	}

	autowiringSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&autowiringOnlySpec{
		Target:          references[0].Ref(),
		EnvironmentType: mapEnvironmentType(context.StateContext.Location.EnvType),
		ServiceName:     context.StateContext.ServiceName,
	})
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	var userSpec map[string]interface{}
	if resource.Spec != nil {
		if err = json.Unmarshal(resource.Spec.Raw, &userSpec); err != nil {
			return nil, false, false, errors.WithStack(err)
		}
	}
	finalSpec, err := wiringutil.Merge(userSpec, autowiringSpec)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	bytes, err := json.Marshal(&finalSpec)
	return bytes, false, false, err
}

func mapEnvironmentType(envType voyager.EnvType) string {
	// DNS provider expects full string of production, rather than "prod"
	if envType == voyager.EnvTypeProduction {
		return "production"
	}

	// currently the other environment types match
	return string(envType)
}

// svccatentangler plugin expects reference function to return a slice of references, in the case of platformdns it will
// always be a single reference.
func getReferences(_ *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]smith_v1.Reference, bool /* externalErr */, bool /* retriable */, error) {
	var references []smith_v1.Reference

	// Ensure we only depend on one resource, as we can only bind to a single ingress
	if len(context.Dependencies) != 1 {
		return nil, true, false, errors.Errorf("%s resources must depend on only one ingress resource", apiplatformdns.ResourceType)
	}
	dependency := context.Dependencies[0]

	ingressShape, found, err := knownshapes.FindIngressEndpointShape(dependency.Contract.Shapes)
	if err != nil {
		return nil, false, false, err
	}
	if !found {
		return nil, true, false, errors.Errorf("shape %q is required to create ServiceBinding for %q but was not found",
			knownshapes.IngressEndpointShape, dependency.Name)
	}
	ingressEndpoint := ingressShape.Data.IngressEndpoint
	referenceName := wiringutil.ReferenceName(ingressEndpoint.Resource, kubeIngressRefMetadata, kubeIngressRefMetadataEndpoint)
	references = append(references, ingressEndpoint.ToReference(referenceName))
	return references, false, false, nil
}
