package knownshapes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/libshapes"
)

const (
	// Exposes a reference to a Deployment object.
	KubeDeploymentShape wiringplugin.ShapeName = "voyager.atl-paas.net/KubeDeploymentShape"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin.Shape
type KubeDeployment struct {
	wiringplugin.ShapeMeta `json:",inline"`
	Data                   KubeDeploymentData `json:"data"`
}

// +k8s:deepcopy-gen=true
type KubeDeploymentData struct {
	// Resource name of the Deployment object.
	DeploymentResourceName smith_v1.ResourceName `json:"deploymentResourceName"`
	// Metadata name of the Deployment object.
	DeploymentName string `json:"deploymentName"`
}

func NewKubeDeployment(resourceName smith_v1.ResourceName, deploymentName string) *KubeDeployment {
	return &KubeDeployment{
		ShapeMeta: wiringplugin.ShapeMeta{
			ShapeName: KubeDeploymentShape,
		},
		Data: KubeDeploymentData{
			DeploymentResourceName: resourceName,
			DeploymentName:         deploymentName,
		},
	}
}

func FindKubeDeploymentShape(shapes []wiringplugin.Shape) (*KubeDeployment, bool /*found*/, error) {
	typed := &KubeDeployment{}
	found, err := libshapes.FindAndCopyShapeByName(shapes, KubeDeploymentShape, typed)
	if err != nil {
		return nil, false, err
	}
	if found {
		return typed, true, nil
	}
	return nil, false, nil
}
