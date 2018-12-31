package monitor

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/atlassian/voyager"
	composition_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
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

func buildServiceDescriptor(name string, location voyager.Location, version string) (*composition_v1.ServiceDescriptor, error) {
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

	return &composition_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: composition_v1.ServiceDescriptorResourceVersion,
			Kind:       composition_v1.ServiceDescriptorResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				replication.ReplicateKey: strconv.FormatBool(false),
			},
		},
		Spec: composition_v1.ServiceDescriptorSpec{
			Locations: []composition_v1.ServiceDescriptorLocation{
				{
					Name:    composition_v1.ServiceDescriptorLocationName(locationName),
					Account: location.Account,
					Region:  location.Region,
					EnvType: location.EnvType,
					Label:   location.Label,
				},
			},
			ResourceGroups: []composition_v1.ServiceDescriptorResourceGroup{
				{
					Name: composition_v1.ServiceDescriptorResourceGroupName(stackName),
					Locations: []composition_v1.ServiceDescriptorLocationName{
						composition_v1.ServiceDescriptorLocationName(locationName),
					},
					Resources: []composition_v1.ServiceDescriptorResource{
						{
							Name: resourceName,
							Type: resourceType,
							Spec: spec,
						},
					},
				},
			},
			Version: version,
		},
	}, nil
}
