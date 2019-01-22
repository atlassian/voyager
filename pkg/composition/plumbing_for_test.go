package composition

import (
	"testing"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func loadTestServiceDescriptor(t *testing.T, filename string) *comp_v1.ServiceDescriptor {
	sd := comp_v1.ServiceDescriptor{}
	sd.Name = "foo"
	sd.Namespace = "bar"
	sd.Spec = comp_v1.ServiceDescriptorSpec{}

	srcFile, err := testutil.LoadFileFromTestData(filename)
	require.NoError(t, err)

	err = yaml.Unmarshal(srcFile, &sd.Spec)
	require.NoError(t, err)

	return &sd
}

// Helper functions
func getFormationObject(foList []FormationObjectInfo, ldName, ldNamespace string) (FormationObjectInfo, error) {
	for _, foItem := range foList {
		if foItem.Name == ldName && foItem.Namespace == ldNamespace {
			return foItem, nil
		}
	}

	return FormationObjectInfo{}, nil
}

func getResource(resourceList []comp_v1.ServiceDescriptorResource, resourceName string, resourceType string) *comp_v1.ServiceDescriptorResource {
	for _, resourceItem := range resourceList {
		if resourceItem.Name == voyager.ResourceName(resourceName) && resourceItem.Type == voyager.ResourceType(resourceType) {
			return &resourceItem
		}
	}

	return nil
}

func readConfig(t *testing.T, filename string) *VarModel {
	sd := loadTestServiceDescriptor(t, filename)
	assert.NotNil(t, sd)

	return createVarModelFromSdSpec(&sd.Spec)
}
