package monitor

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/replication"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	locationName = "default"

	stackName = "ups"

	resourceName voyager.ResourceName = "ups"
	resourceType voyager.ResourceType = "UPS"

	versionParameter = "version"
	timeParameter    = "time"

	pollDelay                        = 5 * time.Second
	serviceDescriptorDeletionTimeout = 1 * time.Minute
)

func buildServiceDescriptor(name string, location voyager.Location, version string) (*comp_v1.ServiceDescriptor, error) {
	bytes, err := json.Marshal(map[string]string{
		versionParameter: version,
		timeParameter:    time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}

	spec := &runtime.RawExtension{
		Raw: bytes,
	}

	return &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: comp_v1.ServiceDescriptorResourceVersion,
			Kind:       comp_v1.ServiceDescriptorResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				replication.ReplicateKey: strconv.FormatBool(false),
			},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				{
					Name:    comp_v1.ServiceDescriptorLocationName(locationName),
					Account: location.Account,
					Region:  location.Region,
					EnvType: location.EnvType,
					Label:   location.Label,
				},
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				{
					Name: comp_v1.ServiceDescriptorResourceGroupName(stackName),
					Locations: []comp_v1.ServiceDescriptorLocationName{
						comp_v1.ServiceDescriptorLocationName(locationName),
					},
					Resources: []comp_v1.ServiceDescriptorResource{
						{
							Name: resourceName,
							Type: resourceType,
							Spec: spec,
						},
					},
				},
			},
		},
	}, nil
}
