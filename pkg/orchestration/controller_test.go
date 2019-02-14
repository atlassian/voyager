package orchestration

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	stateclient_fake "github.com/atlassian/voyager/pkg/orchestration/client/fake"
	"github.com/atlassian/voyager/pkg/orchestration/wiring"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/util/clock"
)

const (
	fixtureStateInputSuffix   = ".state.input.yaml"
	fixtureBundleOutputSuffix = ".bundle.output.yaml"
	fixtureStateOutputSuffix  = ".state.output.yaml"
	fixtureGlob               = "*" + fixtureStateInputSuffix

	testAccount = "testaccount"
	testEnv     = "testenv"
	testRegion  = "testregion"
)

func testHandleProcessResult(t *testing.T, filePrefix string) {
	state := &orch_v1.State{}
	err := testutil.LoadIntoStructFromTestData(filePrefix+fixtureStateInputSuffix, state)
	require.NoError(t, err)
	bundle := &smith_v1.Bundle{}
	err = testutil.LoadIntoStructFromTestData(filePrefix+fixtureBundleOutputSuffix, bundle)
	require.NoError(t, err)

	// Run the processing
	stateTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "test",
			Name:      "state_transitions_total",
			Help:      "Records the number of times a State transitions into a new condition",
		},
		[]string{"namespace", "name", "type", "reason"},
	)
	stateClient := stateclient_fake.NewSimpleClientset(state).OrchestrationV1()
	logger := zaptest.NewLogger(t)
	c := &Controller{
		Logger:                  logger,
		Clock:                   clock.NewFakeClock(time.Unix(0, 0)),
		StateClient:             stateClient,
		StateTransitionsCounter: stateTransitionsCounter,
		Entangler:               entanglerForTests(),
	}

	_, _, err = c.handleProcessResult(logger, state, bundle, false, nil)
	assert.NoError(t, err)

	// Compare the outputs
	fileName := filePrefix + fixtureStateOutputSuffix
	stateExpected := &orch_v1.State{}
	err = testutil.LoadIntoStructFromTestData(fileName, stateExpected)
	require.NoError(t, err)

	testutil.ObjectCompareContext(t, testutil.FileName(fileName), state, stateExpected)
}

func TestOrchestrationWithTestData(t *testing.T) {
	t.Parallel()

	files, errRead := filepath.Glob(filepath.Join(testutil.FixturesDir, fixtureGlob))
	require.NoError(t, errRead)

	// Sanity check that we actually loaded something otherwise bazel might eat
	// our tests
	if len(files) == 0 {
		require.FailNow(t, "Expected some test fixtures, but didn't fine any")
	}

	for _, file := range files {
		_, filename := filepath.Split(file)
		bundleFileName := strings.Split(filename, ".")
		resultFilePrefix := strings.Join(bundleFileName[:len(bundleFileName)-3], ".")

		t.Run(resultFilePrefix, func(t *testing.T) {
			testHandleProcessResult(t, resultFilePrefix)
		})
	}
}

func TestHandleProcessResultDoesNotUpdateIfNoChange(t *testing.T) {
	t.Parallel()

	state := &orch_v1.State{}
	err := testutil.LoadIntoStructFromTestData("test_resource_no_update.state.yaml", state)
	require.NoError(t, err)
	existingBundle := &smith_v1.Bundle{}
	err = testutil.LoadIntoStructFromTestData("test_resource_no_update.bundle.yaml", existingBundle)
	require.NoError(t, err)

	// Run the processing
	stateTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "test",
			Name:      "state_transitions_total",
			Help:      "Records the number of times a State transitions into a new condition",
		},
		[]string{"namespace", "name", "type", "reason"},
	)

	client := stateclient_fake.NewSimpleClientset(state)
	stateClient := client.OrchestrationV1()
	logger := zaptest.NewLogger(t)
	c := &Controller{
		Logger:                  logger,
		Clock:                   clock.NewFakeClock(time.Unix(0, 0)),
		StateClient:             stateClient,
		StateTransitionsCounter: stateTransitionsCounter,
		Entangler:               entanglerForTests(),
	}

	_, _, err = c.handleProcessResult(logger, state, existingBundle, false, nil)
	assert.NoError(t, err)

	// Verify actions
	actions := client.Actions()
	assert.Len(t, actions, 0)
}

func TestHandleProcessResultUpdatesIfResourcesChange(t *testing.T) {
	t.Parallel()

	state := &orch_v1.State{}
	err := testutil.LoadIntoStructFromTestData("test_resource_update.state.yaml", state)
	require.NoError(t, err)
	existingBundle := &smith_v1.Bundle{}
	err = testutil.LoadIntoStructFromTestData("test_resource_update.bundle.yaml", existingBundle)
	require.NoError(t, err)

	// Run the processing
	stateTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "test",
			Name:      "state_transitions_total",
			Help:      "Records the number of times a State transitions into a new condition",
		},
		[]string{"namespace", "name", "type", "reason"},
	)

	client := stateclient_fake.NewSimpleClientset(state)
	stateClient := client.OrchestrationV1()
	logger := zaptest.NewLogger(t)
	c := &Controller{
		Logger:                  logger,
		Clock:                   clock.NewFakeClock(time.Unix(0, 0)),
		StateClient:             stateClient,
		StateTransitionsCounter: stateTransitionsCounter,
		Entangler:               entanglerForTests(),
	}

	_, _, err = c.handleProcessResult(logger, state, existingBundle, false, nil)
	assert.NoError(t, err)

	// Verify actions
	actions := client.Actions()
	assert.Len(t, actions, 1) // 0: update

	assert.Equal(t, "update", actions[0].GetVerb())
	assert.Equal(t, "bh-demo-test", actions[0].GetNamespace())
	assert.Equal(t, orch_v1.SchemeGroupVersion.WithResource(orch_v1.StateResourcePlural), actions[0].GetResource())
}

func TestStateResourceName(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		Input  smith_v1.ResourceName
		Output voyager.ResourceName
	}{
		{"name", "name"},
		{"name--junk", "name"},
	}
	for _, test := range tests {
		assert.Equal(t, stateResourceName(test.Input), test.Output)
	}
}

func entanglerForTests() *wiring.Entangler {
	return &wiring.Entangler{
		Plugins: registry.KnownWiringPlugins(
			testDeveloperRole,
			testManagedPolicies,
			testVPC,
			testEnvironment,
		),
		ClusterLocation: voyager.ClusterLocation{
			Account: testAccount,
			Region:  testRegion,
			EnvType: testEnv,
		},
		ClusterConfig: wiringplugin.ClusterConfig{
			ClusterDomainName: "internal.ap-southeast-2.kitt-integration.kitt-inf.net",
			KittClusterEnv:    "test",
			Kube2iamAccount:   "test",
		},
		Tags: testingTags,
	}
}

func testDeveloperRole(_ voyager.Location) []string {
	return []string{"arn:aws:iam::123456789012:role/micros-server-iam-MicrosServer-ABC"}
}

func testManagedPolicies(_ voyager.Location) []string {
	return []string{"arn:aws:iam::123456789012:policy/SOX-DENY-IAM-CREATE-DELETE", "arn:aws:iam::123456789012:policy/micros-iam-DefaultServicePolicy-ABC"} // example
}

func testVPC(location voyager.Location) *oap.VPCEnvironment {
	return oap.ExampleVPC(location.Label, location.Region)
}

func testEnvironment(_ voyager.Location) string {
	return "microstestenv"
}

func testingTags(
	_ voyager.ClusterLocation,
	_ wiringplugin.ClusterConfig,
	_ voyager.Location,
	_ voyager.ServiceName,
	_ orch_meta.ServiceProperties,
) map[voyager.Tag]string {
	tags := make(map[voyager.Tag]string)
	return tags
}
