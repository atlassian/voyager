package internaldns

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/svccatentangler"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ResourceType                       voyager.ResourceType = "InternalDNS"
	clusterServiceClassExternalID                           = "f77e1881-36f3-42ce-9848-7a811b421dd7"
	clusterServicePlanExternalID                            = "0a7b1d18-cf8d-461e-ad24-ee16d3da36d3"
	kubeIngressResourceType            voyager.ResourceType = "KubeIngress"
	kubeIngressRefMetadata                                  = "metadata"
	kubeIngressRefMetadataEndpoint                          = "endpoint"
	kubeIngressRefMetadataEndpointPath                      = "metadata.annotations['atlassian\\.com/ingress\\.endpoint']"
	kubeIngressRefExample                                   = "ingress-internal-01.ap-southeast-2.paas-dev1.kitt-inf.net"
)

type Alias struct {
	AliasType string `json:"type"`
	Name      string `json:"name"`
}

type UserSpec struct {
	Aliases []Alias `json:"aliases"`
}

type autowiringOnlySpec struct {
	Target          string              `json:"target"`
	ServiceName     voyager.ServiceName `json:"serviceName"`
	EnvironmentType string              `json:"environmentType"`
}

type WiringPlugin struct {
	svccatentangler.SvcCatEntangler
}

func New() *WiringPlugin {
	return &WiringPlugin{
		SvcCatEntangler: svccatentangler.SvcCatEntangler{
			ClusterServiceClassExternalID: clusterServiceClassExternalID,
			ClusterServicePlanExternalID:  clusterServicePlanExternalID,
			InstanceSpec:                  getInstanceSpec,
			ObjectMeta:                    getObjectMeta,
			References:                    getReferences,
			ResourceType:                  ResourceType,
			OptionalShapes:                svccatentangler.NoOptionalShapes,
		},
	}
}

// can only depend on one kubeIngress
func getIngressDependency(dependencies []wiringplugin.WiredDependency) (wiringplugin.WiredDependency, error) {
	if len(dependencies) == 1 {
		if dependencies[0].Type == kubeIngressResourceType {
			return dependencies[0], nil
		}
	}
	return wiringplugin.WiredDependency{}, errors.Errorf("internaldns resources must depend on only one ingress resource")
}

func getInstanceSpec(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]byte, error) {

	// Don't allow user to set anything they shouldn't
	if resource.Spec != nil {
		var ourSpec autowiringOnlySpec
		if err := json.Unmarshal(resource.Spec.Raw, &ourSpec); err != nil {
			return nil, errors.WithStack(err)
		}
		if ourSpec != (autowiringOnlySpec{}) {
			return nil, errors.Errorf("unsupported parameters were provided: %+v", ourSpec)
		}
	}

	references, err := getReferences(resource, context)
	if err != nil {
		return nil, err
	}

	autowiringSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&autowiringOnlySpec{
		Target:          references[0].Ref(),
		EnvironmentType: mapEnvironmentType(context.StateContext.Location.EnvType),
		ServiceName:     context.StateContext.ServiceName,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var userSpec map[string]interface{}
	if resource.Spec != nil {
		if err = json.Unmarshal(resource.Spec.Raw, &userSpec); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	finalSpec, err := wiringutil.Merge(userSpec, autowiringSpec)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return json.Marshal(&finalSpec)

}

func mapEnvironmentType(envType voyager.EnvType) string {
	// DNS provider expects full string of production, rather than "prod"
	if envType == voyager.EnvTypeProduction {
		return "production"
	}

	// currently the other environment types match
	return string(envType)
}

func getReferenceNameByGVK(smithResources []smith_v1.Resource, gvk schema.GroupVersionKind) (smith_v1.ResourceName, error) {
	for _, smithReference := range smithResources {
		if smithReference.Spec.Object.GetObjectKind().GroupVersionKind() == gvk {
			return smithReference.Name, nil
		}
	}
	return "", errors.New("could not find smith resource for requested GVK")
}

// svccatentangler plugin expects reference function to return a slice of reference
// in this internaldns case, it will always be only one reference, because it's limited by getIngressDependency method
func getReferences(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) ([]smith_v1.Reference, error) {
	var references []smith_v1.Reference
	// there can be only one ingressDependency
	ingressDependency, err := getIngressDependency(context.Dependencies)
	if err != nil {
		return nil, err
	}
	ingressResourceReferenceName, err := getReferenceNameByGVK(ingressDependency.SmithResources, schema.GroupVersionKind{
		Group:   "extensions",
		Version: "v1beta1",
		Kind:    k8s.IngressKind,
	})
	if err != nil {
		return nil, err
	}
	referenceName := wiringutil.ReferenceName(ingressResourceReferenceName, kubeIngressRefMetadata, kubeIngressRefMetadataEndpoint)
	references = append(references, smith_v1.Reference{
		Name:     referenceName,
		Resource: ingressResourceReferenceName,
		Path:     kubeIngressRefMetadataEndpointPath,
		Example:  kubeIngressRefExample,
	})
	return references, nil
}

func getObjectMeta(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (meta_v1.ObjectMeta, error) {
	return meta_v1.ObjectMeta{
		Annotations: map[string]string{
			voyager.Domain + "/envResourcePrefix": string(ResourceType),
		},
	}, nil
}
