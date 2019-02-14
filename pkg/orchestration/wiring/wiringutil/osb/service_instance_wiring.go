package osb

import (
	"encoding/json"

	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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
		return "", errors.Wrap(err, "unmarshalling StateResource spec failed")
	}

	return spec.InstanceID, nil
}

func ConstructServiceInstance(resource *orch_v1.StateResource, classID servicecatalog.ClassExternalID, planID servicecatalog.PlanExternalID) (*sc_v1b1.ServiceInstance, error) {
	serviceInstanceExternalID, err := InstanceID(resource.Spec)
	if err != nil {
		return nil, err
	}

	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: wiringutil.ServiceInstanceMetaName(resource.Name),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalID: string(classID),
				ClusterServicePlanExternalID:  string(planID),
			},
			ExternalID: serviceInstanceExternalID,
		},
	}, nil
}
