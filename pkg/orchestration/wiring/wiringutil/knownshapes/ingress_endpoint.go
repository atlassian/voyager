package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
)

const (
	// Exposes reference to ingress endpoint details within source resource (KubeCompute initially)
	IngressEndpointShape wiringplugin.ShapeName = "voyager.atl-paas.net/IngressEndpointShape"

	kubeIngressRefMetadataEndpointPath = "metadata.annotations['atlassian\\.com/ingress\\.endpoint']"
	kubeIngressRefExample              = "ingress-internal-01.ap-southeast-2.paas-dev1.kitt-inf.net"
	kubeIngressReferenceEndpointSuffix = "endpoint"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type IngressEndpoint struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   IngressEndpointData `json:"data"`
}

// +k8s:deepcopy-gen=true
type IngressEndpointData struct {
	IngressEndpoint libshapes.ProtoReference `json:"ingressEndpoint"`
}

func NewIngressEndpoint(resourceName smith_v1.ResourceName) *IngressEndpoint {
	return &IngressEndpoint{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: IngressEndpointShape,
		},
		Data: IngressEndpointData{
			IngressEndpoint: libshapes.ProtoReference{
				Resource:    resourceName,
				Path:        kubeIngressRefMetadataEndpointPath,
				Example:     kubeIngressRefExample,
				NamePostfix: kubeIngressReferenceEndpointSuffix,
			},
		},
	}
}

func FindIngressEndpointShape(shapes []wiringplugin.Shape) (*IngressEndpoint, bool /*found*/, error) {
	typed := &IngressEndpoint{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, IngressEndpointShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
