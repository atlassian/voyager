package edge

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orchestration "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/osb"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "GlobalEdge"
	serviceID                         = "10e5a402-45df-5afd-ae86-11377ce2bbb2"
	planID                            = "7d57270a-0348-58d3-829d-447a98fe98d5"
)

type WiringPlugin struct {
}

func New() *WiringPlugin {
	return &WiringPlugin{}
}

func (p *WiringPlugin) WireUp(resource *orchestration.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	serviceInstance, err := osb.ConstructServiceInstance(resource, serviceID, planID)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	instanceParameters, err := instanceParameters(resource, context)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	serviceInstance.Spec.Parameters = &runtime.RawExtension{
		Raw: instanceParameters,
	}

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: nil, // Nothing will depend on GlobalEdge
		},
		Resources: []smith_v1.Resource{
			{
				Name:       wiringutil.ServiceInstanceResourceName(resource.Name),
				References: nil,
				Spec: smith_v1.ResourceSpec{
					Object: serviceInstance,
				},
			},
		},
	}
}

func instanceParameters(resource *orchestration.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {
	if resource.Spec == nil {
		return nil, errors.New("empty spec is not allowed")
	}

	// Pass-through SD parameters -> OSB attributes
	var spec Spec
	if err := json.Unmarshal(resource.Spec.Raw, &spec); err != nil {
		return nil, errors.WithStack(err)
	}

	if len(spec.UpstreamAddresses) == 0 {
		return nil, errors.New("UpstreamAddresses must not be empty")

		// TODO: Make upstream address optional and produce it from EC2Compute / KubeIngress output
		// 1. Find dependency of some expected shape ("UpstreamAddressProviderShape" or whatever)
		// 2. Generate a reference to the field from that dependency to be used as an upstream address
		// 3. set UpstreamAddresses to this reference inside parameters, i.e.
		// 	spec.UpstreamAddresses = bla
	}

	parameters := specToParameters(&spec, context)
	return json.Marshal(parameters)
}

func specToParameters(spec *Spec, context *wiringplugin.WiringContext) *osbInstanceParameters {
	attributes := osbAttributes{
		UpstreamAddress: convertUpstreamAddresses(spec.UpstreamAddresses),
		UpstreamPort:    spec.UpstreamPort,
		UpstreamSuffix:  spec.UpstreamSuffix,
		UpstreamOnly:    spec.UpstreamOnly,
		Domain:          spec.Domains,
		Healthcheck:     spec.Healthcheck,
		Rewrite:         spec.Rewrite,
		Routes:          convertRoutes(spec.Routes),
	}

	return &osbInstanceParameters{
		ServiceName: context.StateContext.ServiceName,
		Resource: osbResourceParameters{
			Attributes: attributes,
		},
	}
}

func convertUpstreamAddresses(addresses []UpstreamAddress) []osbUpstreamAddress {
	osbAddresses := make([]osbUpstreamAddress, 0, len(addresses))
	for _, address := range addresses {
		osbAddresses = append(osbAddresses, osbUpstreamAddress(address))
	}
	return osbAddresses
}

func convertRoutes(routes []Route) []osbRoute {
	osbRoutes := make([]osbRoute, 0, len(routes))
	for _, route := range routes {
		osbRoute := osbRoute{
			Match: osbRouteMatch{
				Prefix: route.Match.Prefix,
				Regex:  route.Match.Regex,
				Path:   route.Match.Path,
				Host:   route.Match.Host,
			},
			Route: osbRouteAction{
				Cluster:       route.Route.Cluster,
				PrefixRewrite: route.Route.PrefixRewrite,
			},
		}
		osbRoutes = append(osbRoutes, osbRoute)
	}
	return osbRoutes
}
