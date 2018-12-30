package composition

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/formation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEmptyResources(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_empty_resources.yml")
	expectedLocation := voyager.Location{
		Region:  voyager.Region("us-west-1"),
		EnvType: voyager.EnvTypeStaging,
		Account: voyager.Account("123"),
		Label:   voyager.Label("foo"),
	}
	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "123", Region: "us-west-1", EnvType: "staging"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)

	require.Len(t, formationObjectList, 1, "Expected only 1 formation object as there is only 1 location")

	fo := formationObjectList[0]
	assert.Empty(t, fo.Resources, "No resources should have been mapped")

	assert.Equal(t, expectedLocation, fo.Location, "Incorrect location mapped")
}

func TestHandlesResourceWithNoSpec(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_spec_not_specified.yml")

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A123", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)

	assert.Nil(t, formationObjectList[0].Resources[0].Spec)
}

func TestLocationValidWithoutLabel(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_label_not_specified.yml")

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	if err != nil {
		assert.Fail(t, fmt.Sprintf("Could not create formation object definition. %v", err))
	}

	expectedDevLocation := comp_v1.ServiceDescriptorLocation{
		Region:  voyager.Region("us-west-1"),
		Account: voyager.Account("A123"),
		Name:    "us-west1-dev",
		Label:   voyager.Label(""),
		EnvType: voyager.EnvTypeDev,
	}

	expectedProdLocation := comp_v1.ServiceDescriptorLocation{
		Region:  voyager.Region("us-west-1"),
		Account: voyager.Account("321A"),
		Name:    "us-west1-prod",
		Label:   voyager.Label("sre"),
		EnvType: voyager.EnvTypeProduction,
	}

	for _, fo := range formationObjectList {
		location := fo.Location

		if location.EnvType == voyager.EnvTypeDev {
			assert.Equal(t, expectedDevLocation, location, "Incorrect region mapped")
			assert.Len(t, fo.Resources, 1, "Incorrect number of resources mapped")
		} else if location.EnvType == voyager.EnvTypeProduction {
			assert.Equal(t, expectedProdLocation, location, "Incorrect region mapped")
			assert.Len(t, fo.Resources, 1, "Incorrect number of resources mapped")
		} else {
			assert.Fail(t, "Incorrect mapping")
		}
	}
}

func TestNoLocationInResourceGroup(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_no_location_in_resource_group.yml")

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.Error(t, err)
}

func TestFoDefNotCreatedIfClusterNotMatched(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_location_doesnt_match_cluster.yml")

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "prod"})

	foList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)
	assert.Empty(t, foList)
}

func TestReadLocation(t *testing.T) {
	t.Parallel()

	sd := loadTestServiceDescriptor(t, "test_read_location.yml")
	require.NotNil(t, sd.Spec)
	require.Len(t, sd.Spec.Locations, 2, "Not all locations were read")

	var prodLocation comp_v1.ServiceDescriptorLocation
	var devLocation comp_v1.ServiceDescriptorLocation

	if sd.Spec.Locations[0].EnvType == "dev" {
		devLocation = sd.Spec.Locations[0]
		prodLocation = sd.Spec.Locations[1]
	} else if sd.Spec.Locations[0].EnvType == "prod" {
		devLocation = sd.Spec.Locations[1]
		prodLocation = sd.Spec.Locations[0]
	} else {
		assert.Fail(t, "WTF")
	}

	assert.Equal(t, comp_v1.ServiceDescriptorLocationName("us-west1-dev"), devLocation.Name, "Incorrect name for location")
	assert.Equal(t, voyager.Region("us-west-1"), devLocation.Region, "Incorrect region for location")
	assert.Equal(t, voyager.Account("A123"), devLocation.Account, "Incorrect account for location")
	assert.Equal(t, voyager.Label("user"), devLocation.Label, "Incorrect label for location")

	assert.Equal(t, comp_v1.ServiceDescriptorLocationName("us-west1-prod"), prodLocation.Name, "Incorrect name for location")
	assert.Equal(t, voyager.Region("us-west-1"), prodLocation.Region, "Incorrect region for location")
	assert.Equal(t, voyager.Account("321A"), prodLocation.Account, "Incorrect account for location")
	assert.Equal(t, voyager.Label("sre"), prodLocation.Label, "Incorrect label for location")

}

func TestVariableSubstitution(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_substitution.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err, "Expected no error when creating formation objects")

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-ddb", "dynamodb")
	assert.NotNil(t, res, "Could not find resource")

	spec, err := specToMap(res.Spec)
	require.NoError(t, err)

	rcu := spec["RCU"]
	assert.Equal(t, 5.0, rcu, "Incorrect value for variable")
}

func TestVariableSubstitutionWithInvalidPrefix(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_substitution_with_invalid_prefix.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err, "expected this to be an invalid prefix")
	require.Contains(t, err.Error(), "was not one of the expected prefixes:")
	require.Nil(t, formationObjectList, "should not produce any formation object")
}

func TestVariableSubstitutionWithInvalidKey(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_substitution_with_invalid_key.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err, "expected this to be an invalid prefix")
	require.Contains(t, err.Error(), "was not one of the expected prefixes:")
	require.Nil(t, formationObjectList, "should not produce any formation object")
}

func TestInlineVariableSubstitution(t *testing.T) {
	t.Parallel()

	expectedSpecValue := map[string]interface{}{
		"element": map[string]interface{}{
			"item1": "reallyNotList",
			"item2": "alsoNotList",
			"item3": "anotherItem",
		},
	}
	serviceDescriptor := loadTestServiceDescriptor(t, "test_inline_variable_substitution.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err, "Could not create formation object")

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-ddb", "dynamodb")
	assert.NotNil(t, res, "Could not find resource")
	assert.NotNil(t, res.Spec, "Resource lost it's spec")
	spec, err := specToMap(res.Spec)
	require.NoError(t, err)
	assert.Equal(t, expectedSpecValue, spec, "Incorrect value for variable")

	res = getResource(fo.Resources, "test-ddb-with-prefix", "dynamodb")
	assert.NotNil(t, res, "Could not find resource")
	assert.NotNil(t, res.Spec, "Resource lost it's spec")
	spec, err = specToMap(res.Spec)
	require.NoError(t, err)
	assert.Equal(t, expectedSpecValue, spec, "Incorrect value for variable")
}

func TestInlineVariableSubstitutionWithReservedVar(t *testing.T) {
	t.Parallel()

	expectedSpecValue := map[string]interface{}{
		"${inline}": "${release:inline-test}", // If it's reserved it should allow the inline block to fall through.
	}
	serviceDescriptor := loadTestServiceDescriptor(t, "test_inline_variable_substitution_w_reserved_key.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err, "Could not create formation object")

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-ddb", "dynamodb")
	require.NotNil(t, res, "Could not find resource")
	require.NotNil(t, res.Spec, "Resource lost it's spec")

	spec, err := specToMap(res.Spec)
	require.NoError(t, err)

	assert.Equal(t, expectedSpecValue, spec, "Incorrect value for variable")
}

func TestVariableSubstitutionIsBackwardsCompatible(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_substitution_is_backwards_compatible.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err, "Should be no error creating formation object")

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-ddb", "dynamodb")
	require.NotNil(t, res, "Could not find resource")

	spec, err := specToMap(res.Spec)
	require.NoError(t, err)

	rcu := spec["RCU"]
	assert.Equal(t, 5.0, rcu, "Incorrect value for variable")

	wcu := spec["WCU"]
	assert.Equal(t, 5.0, wcu, "Incorrect value for variable")
}

func TestLabelNotInLocation(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_label_not_in_location.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	fo, err := getFormationObject(formationObjectList, "foo", "foo")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-resource", "tester")
	assert.NotNil(t, res, "Could not find resource")

	spec, err := specToMap(res.Spec)
	require.NoError(t, err)

	rcu := spec["msg"]
	assert.Equal(t, "found-unspecifiedLabel", rcu, "Incorrect value for variable")
}

func TestOneLocationMultipleResourceGroups(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_one_location_multiple_resource_groups.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A123", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)

	fo, err := getFormationObject(formationObjectList, "foo--user", "foo--user")
	require.NoError(t, err)

	res := getResource(fo.Resources, "first-resource", "some-type")
	assert.NotNil(t, res)

	res = getResource(fo.Resources, "second-resource", "other-type")
	assert.NotNil(t, res)

}

func TestDuplicateResourceNameWithinResourceGroup(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_dupe_resource_name_same_resource_group.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "123", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
	assert.Contains(t, err.Error(), serviceDescriptor.Spec.ResourceGroups[0].Resources[0].Name)
}

func TestDuplicateResourceNameAcrossResourceGroups(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_dupe_resource_name_across_resource_group.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "123", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
	assert.Contains(t, err.Error(), serviceDescriptor.Spec.ResourceGroups[0].Resources[0].Name)
}

func TestDependsOnNotThere(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_dependson_not_there.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "123", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
	assert.Contains(t, err.Error(), serviceDescriptor.Spec.ResourceGroups[0].Resources[0].DependsOn[0].Name)
}

func TestDependsOnItself(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_dependson_itself.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "123", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
	assert.Contains(t, err.Error(), serviceDescriptor.Spec.ResourceGroups[0].Resources[0].DependsOn[0].Name)
}

func TestVariableDoesNotExistInMapReturnsError(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_does_not_exist_in_map.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
}

func TestVariableDoesNotExistInListReturnsError(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_does_not_exist_in_list.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)

	require.Error(t, err)
}

func TestVariableWithWithReservedPrefixDoesNotReturnError(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_substitution_with_a_reserved_prefix.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	res := getResource(fo.Resources, "test-ddb", "dynamodb")
	assert.NotNil(t, res, "Could not find resource")

	spec, err := specToMap(res.Spec)

	require.NoError(t, err)
	wcu := spec["WCU"]
	assert.Equal(t, 5.0, wcu, "Incorrect value for variable")

	rcu := spec["RCU"]
	assert.Equal(t, fmt.Sprintf("${%ssomething.else}", formation.ReleaseTemplatingPrefix), rcu, "Should have preserved value with reserved prefix for later processing.")
}

func TestVariableConcatenation(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_variable_concatenation.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	formationObjectList, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)

	fo, err := getFormationObject(formationObjectList, "foo--myLabel", "foo--myLabel")
	require.NoError(t, err, "Could not find formation object")

	compute := getResource(fo.Resources, "compute", "KubeCompute")
	assert.NotNil(t, compute, "Could not find resource")
	computeSpec, err := specToMap(compute.Spec)
	require.NoError(t, err)

	proxy := getResource(fo.Resources, "proxy", "KubeCompute")
	assert.NotNil(t, compute, "Could not find resource")
	proxySpec, err := specToMap(proxy.Spec)
	require.NoError(t, err)

	computeImage := computeSpec["containers"].(map[string]interface{})["image"]
	expectedValue := `${release:myservice.compute.image}@${release:myservice.compute.digest}`
	assert.Equal(t, expectedValue, computeImage, "Should have preserved concatenated release prefixed for later processing.")

	proxyImage := proxySpec["containers"].(map[string]interface{})["image"]
	assert.Equal(t, "docker.example.com/path/myservice-proxy:2.0.0", proxyImage, "Should have expanded and concatenated both config variables.")
}

func TestMultipleLocationsSameResourceDoesNotReturnError(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_multiple_locations_same_resource.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.NoError(t, err)
}

func TestMultipleLocationsSameNameReturnsError(t *testing.T) {
	t.Parallel()

	serviceDescriptor := loadTestServiceDescriptor(t, "test_multiple_locations_same_name.yml")
	require.NotNil(t, serviceDescriptor)

	transformer := NewServiceDescriptorTransformer(voyager.ClusterLocation{Account: "A234", Region: "us-west-1", EnvType: "dev"})
	_, err := transformer.CreateFormationObjectDef(serviceDescriptor)
	require.EqualError(t, err, `resource "first-resource" appears multiple times for the same location`)
}

func specToMap(spec *runtime.RawExtension) (map[string]interface{}, error) {
	var res map[string]interface{}

	err := json.Unmarshal(spec.Raw, &res)

	return res, err
}
