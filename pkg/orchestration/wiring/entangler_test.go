package wiring

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager"
	smith_config "github.com/atlassian/voyager/cmd/smith/config"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/util/layers"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const (
	fixtureStateYamlSuffix      = ".state.yaml"
	fixtureBundleYamlSuffix     = ".bundle.yaml"
	fixtureErrorYamlSuffix      = ".error"
	fixtureErrorRegexYamlSuffix = ".errorregex"
	fixtureGlob                 = "*" + fixtureStateYamlSuffix
)

func TestEntangler(t *testing.T) {
	t.Parallel()

	files, errRead := filepath.Glob(filepath.Join(testutil.FixturesDir, fixtureGlob))
	require.NoError(t, errRead)

	// Sanity check that we actually loaded something otherwise bazel might eat
	// our tests
	if len(files) == 0 {
		require.FailNow(t, "Expected some test fixtures, but didn't fine any")
	}

	for _, file := range files {
		// Given something like "a.b.c.state.yaml" this spits out the prefix "a.b.c"
		_, filename := filepath.Split(file)
		bundleFileName := strings.Split(filename, ".")
		resultFilePrefix := strings.Join(bundleFileName[:len(bundleFileName)-2], ".")

		// Runs the text for that fixture
		t.Run(resultFilePrefix, func(t *testing.T) {
			testFixture(t, resultFilePrefix)
		})
	}
}

func TestEntanglerWithBadWiringFunction(t *testing.T) {
	t.Parallel()

	wireup := registry.WireUpFunc(func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool, error) {
		return &wiringplugin.WiringResult{
			Contract: wiringplugin.ResourceContract{
				Shapes: []wiringplugin.Shape{
					knownshapes.NewASAPKey(),
					knownshapes.NewASAPKey(),
				},
			},
			Resources: nil,
		}, false, nil
	})

	plugins := map[voyager.ResourceType]wiringplugin.WiringPlugin{
		"DoubleASAP": wireup,
	}

	state := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "State",
			APIVersion: "orchestration.voyager.atl-paas.net/v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "state1",
			Namespace: "namespace1",
		},
		Spec: orch_v1.StateSpec{
			Resources: []orch_v1.StateResource{
				{
					Name: "resource1",
					Type: "DoubleASAP",
				},
			},
		},
	}

	_, _, err := entangleTestState(t, state, plugins)

	require.Error(t, err)

}

func TestDependants(t *testing.T) {
	t.Parallel()

	const (
		childFieldKey                        = "child_field"
		childFieldValue                      = "this is a field inside the child of the parent"
		parentName      voyager.ResourceName = "parent"
		childName       voyager.ResourceName = "child"
		parentType      voyager.ResourceType = "ParentType"
		childType       voyager.ResourceType = "ChildType"
	)

	attrs := map[string]interface{}{
		"x": int64(42),
	}

	// Build the parent func, which will need to be able to access the child spec
	var parentFunc registry.WireUpFunc = func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (r *wiringplugin.WiringResult, retriable bool, e error) {
		// ensure that the dependents slice is not empty
		require.Len(t, context.Dependants, 1)
		dependantResource := context.Dependants[0]

		// ensure that the child is actually passed as the dependant resource to the parent
		assert.Equal(t, childName, dependantResource.Name)
		assert.Equal(t, attrs, dependantResource.Attributes)

		// unmarshal the spec
		var spec map[string]string
		err := json.Unmarshal(dependantResource.Resource.Spec.Raw, &spec)
		require.NoError(t, err)

		// ensure the parent can access the data in the spec
		value, ok := spec[childFieldKey]
		assert.True(t, ok)
		assert.Equal(t, childFieldValue, value)

		return &wiringplugin.WiringResult{}, false, nil
	}

	// Child spec, does nothing
	var childFunc registry.WireUpFunc = func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (r *wiringplugin.WiringResult, retriable bool, e error) {
		return &wiringplugin.WiringResult{}, false, nil
	}

	wiringPlugins := map[voyager.ResourceType]wiringplugin.WiringPlugin{
		parentType: parentFunc,
		childType:  childFunc,
	}

	// Build the child's spec which we will have access to in the parent
	childSpec, err := json.Marshal(map[string]string{
		childFieldKey: childFieldValue,
	})
	require.NoError(t, err)

	state := &orch_v1.State{
		Spec: orch_v1.StateSpec{
			Resources: []orch_v1.StateResource{
				{
					Name: parentName,
					Type: parentType,
				},
				{
					Name: childName,
					Type: childType,
					DependsOn: []orch_v1.StateDependency{
						{
							Name:       parentName,
							Attributes: attrs,
						},
					},
					Spec: &runtime.RawExtension{Raw: childSpec},
				},
			},
		},
	}

	_, retriable, err := entangleTestState(t, state, wiringPlugins)
	require.NoError(t, err)
	assert.False(t, retriable)
}

func testFixture(t *testing.T, filePrefix string) {
	// Load and validate expected bundle
	fileName := filePrefix + fixtureBundleYamlSuffix
	bundleExpected := &smith_v1.Bundle{}
	errSuccess := testutil.LoadIntoStructFromTestData(fileName, bundleExpected)
	if errSuccess == nil {
		validateBundle(t, fileName, bundleExpected)
	}

	bundleActual, retriable, err := entangleTestFileState(t, filePrefix)

	// Compare the output
	if errSuccess == nil {
		require.NoError(t, err)
		assert.False(t, retriable)
		testutil.BundleCompareContext(t, testutil.FileName(fileName), bundleExpected, bundleActual)
	}

	data, errFailure := testutil.LoadFileFromTestData(filePrefix + fixtureErrorYamlSuffix)
	if errFailure == nil {
		require.EqualError(t, err, strings.TrimSpace(string(data)))
	}

	rawRegex, errRegexFailure := testutil.LoadFileFromTestData(filePrefix + fixtureErrorRegexYamlSuffix)
	if errRegexFailure == nil {
		require.Regexp(t, rawRegex, err)
	}

	if errFailure != nil && errRegexFailure != nil && errSuccess != nil {
		t.Errorf("Must have either error or bundle file for input %q (%+v, %+v, %+v)", filePrefix, errFailure, errRegexFailure, errSuccess)
	}
}

func validateBundle(t *testing.T, bundleName string, bundle *smith_v1.Bundle) {
	pluginContainers := makePluginContainers(t)
	for _, resource := range bundle.Spec.Resources {
		plugin := resource.Spec.Plugin
		if plugin != nil {
			pluginContainer := pluginContainers[plugin.Name]
			validationResult, err := pluginContainer.ValidateSpec(plugin.Spec)
			require.NoErrorf(t, err, "Validating %s failed: validation of resource %s yielded an error", bundleName, resource.Name)
			assert.Zero(t, validationResult.Errors, "Validating %s failed: resource %s has invalid plugin %s spec", bundleName, resource.Name, plugin.Name)
		}
	}
}

func makePluginContainers(t *testing.T) map[smith_v1.PluginName]smith_plugin.Container {
	smithPlugins := smith_config.Plugins()
	var containers = make(map[smith_v1.PluginName]smith_plugin.Container, len(smithPlugins))
	for _, pluginNewFunc := range smithPlugins {
		container, err := smith_plugin.NewContainer(pluginNewFunc)
		require.NoError(t, err)
		containers[container.Plugin.Describe().Name] = container
	}
	return containers
}

func entangleTestState(t *testing.T, state *orch_v1.State, wiringPlugins map[voyager.ResourceType]wiringplugin.WiringPlugin) (*smith_v1.Bundle, bool, error) {
	// Run the entangle
	ent := Entangler{
		Plugins: wiringPlugins,
		ClusterLocation: voyager.ClusterLocation{
			Account: legacy.TestAccountName,
			Region:  legacy.TestRegion,
			EnvType: legacy.TestEnvironment,
		},
		ClusterConfig: wiringplugin.ClusterConfig{
			ClusterDomainName: "internal.ap-southeast-2.kitt-integration.kitt-inf.net",
			KittClusterEnv:    "test",
			Kube2iamAccount:   "test",
		},
		TagNames: TagNames{
			ServiceNameTag:     "service_name",
			BusinessUnitTag:    "business_unit",
			ResourceOwnerTag:   "resource_owner",
			PlatformTag:        "platform",
			EnvironmentTypeTag: "environment_type",
		},
		GetLegacyConfigFunc: getTestLegacyConfig,
	}
	labels := state.GetLabels()
	namespace := core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Labels: map[string]string{
				voyager.ServiceNameLabel: "test-servicename",
				// This is just to allow fixtures to specify Label. In reality Label is only supported on Namespaces. See below.
				voyager.ServiceLabelLabel: labels[voyager.ServiceLabelLabel],
			},
		},
	}
	delete(labels, voyager.ServiceLabelLabel)
	state.SetLabels(labels)
	serviceName, err := layers.ServiceNameFromNamespaceLabels(namespace.Labels)
	require.NoError(t, err)
	bundle, retriable, err := ent.Entangle(state, &EntanglerContext{
		ServiceName: serviceName,
		Label:       layers.ServiceLabelFromNamespaceLabels(namespace.Labels),
		ServiceProperties: orch_meta.ServiceProperties{
			ResourceOwner: "an_owner",
			BusinessUnit:  "some_unit",
			Notifications: orch_meta.Notifications{
				Email: "an_owner@example.com",
				LowPriorityPagerdutyEndpoint: orch_meta.PagerDuty{
					CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/12312312312312312312312312312312",
					Generic:    "123123123123123",
				},
				PagerdutyEndpoint: orch_meta.PagerDuty{
					CloudWatch: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/12312312312312312312312312312312",
					Generic:    "123123123123123",
				},
			},
			SSAMAccessLevel: "access-level-from-configmap",
			LoggingID:       "logging-id-from-configmap",
		},
	})
	return bundle, retriable, err
}

func entangleTestFileState(t *testing.T, filePrefix string) (*smith_v1.Bundle, bool, error) {
	t.Logf("testFixture prefix: %q\n", filePrefix)

	state := &orch_v1.State{}
	err := testutil.LoadIntoStructFromTestData(filePrefix+fixtureStateYamlSuffix, state)
	require.NoError(t, err)

	return entangleTestState(t, state, registry.KnownWiringPlugins)
}

func getTestLegacyConfig(location voyager.Location) *legacy.Config {
	return legacy.GetLegacyConfigFromMap(legacy.TestLegacyConfigs, location)
}

// In order to replace all expected bundles with actual bundles:
// 1. Remove prefix _
// 2. Run this single test
// 3. Add prefix _ back
func _TestDumpActualBundleToFixtures(t *testing.T) {
	files, errRead := filepath.Glob(filepath.Join(testutil.FixturesDir, fixtureGlob))
	require.NoError(t, errRead)

	// Sanity check that we actually loaded something otherwise bazel might eat
	// our tests
	if len(files) == 0 {
		require.FailNow(t, "Expected some test fixtures, but didn't find any")
	}

	for _, file := range files {
		// Given something like "a.b.c.state.yaml" this spits out the prefix "a.b.c"
		var resultFilePrefix string
		{
			_, filename := filepath.Split(file)
			bundleFileName := strings.Split(filename, ".")
			resultFilePrefix = strings.Join(bundleFileName[:len(bundleFileName)-2], ".")
		}

		writeFixture(t, resultFilePrefix)
	}
}

func writeFixture(t *testing.T, filePrefix string) {
	bundleActual, _, err := entangleTestFileState(t, filePrefix)

	// Compare the output
	fileName := filePrefix + fixtureBundleYamlSuffix
	if err == nil {
		writeBundleToTestData(t, fileName, bundleActual)
	}
}

func writeBundleToTestData(t *testing.T, filename string, bundle *smith_v1.Bundle) {
	data, err := yaml.Marshal(bundle)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(testutil.FixturesDir, filename), data, 0644)
	require.NoError(t, err)
}
