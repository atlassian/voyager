package formation

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/formation/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	formclient_fake "github.com/atlassian/voyager/pkg/formation/client/fake"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	formUpdater "github.com/atlassian/voyager/pkg/formation/updater"
	"github.com/atlassian/voyager/pkg/k8s"
	orchclient_fake "github.com/atlassian/voyager/pkg/orchestration/client/fake"
	orchInf "github.com/atlassian/voyager/pkg/orchestration/informer"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	k8s_fake "k8s.io/client-go/kubernetes/fake"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/yaml"
)

const (
	fixtureLocationDescriptorInputSuffix  = ".ld.input.yaml"
	fixtureStateOutputSuffix              = ".state.output.yaml"
	fixtureLocationDescriptorOutputSuffix = ".ld.output.yaml"
	fixtureGlob                           = "*" + fixtureLocationDescriptorInputSuffix
	testConfigMapNamespace                = "testNamespace"
)

func testHandleProcessResult(t *testing.T, filePrefix string) {
	ld := &form_v1.LocationDescriptor{}
	err := testutil.LoadIntoStructFromTestData(filePrefix+fixtureLocationDescriptorInputSuffix, ld)
	require.NoError(t, err)
	state := &orch_v1.State{}
	err = testutil.LoadIntoStructFromTestData(filePrefix+fixtureStateOutputSuffix, state)
	require.NoError(t, err)

	// Run the processing
	ldTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "test",
			Name:      "ld_transitions_total",
			Help:      "Test Counter",
		},
		[]string{"namespace", "name", "type", "reason"},
	)
	formClient := formclient_fake.NewSimpleClientset(ld).FormationV1()
	logger := zaptest.NewLogger(t)
	c := &Controller{
		LDTransitionsCounter: ldTransitionsCounter,
		Logger:               logger,
		Clock:                clock.NewFakeClock(time.Unix(0, 0)),
		LDClient:             formClient,
	}

	_, _, err = c.handleProcessResult(logger, ld, state, false, nil)
	assert.NoError(t, err)

	// Compare the outputs
	fileName := filePrefix + fixtureLocationDescriptorOutputSuffix
	ldExpected := &form_v1.LocationDescriptor{}
	err = testutil.LoadIntoStructFromTestData(fileName, ldExpected)
	require.NoError(t, err)
	testutil.ObjectCompareContext(t, testutil.FileName(fileName), ld, ldExpected)
}

func TestFormationWithTestData(t *testing.T) {
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
		stateFileName := strings.Split(filename, ".")
		resultFilePrefix := strings.Join(stateFileName[:len(stateFileName)-3], ".")

		t.Run(resultFilePrefix, func(t *testing.T) {
			testHandleProcessResult(t, resultFilePrefix)
		})
	}
}
func TestCreatesStateIncludesOwnerReference(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			UID:       "some-uid",
			Namespace: "parent-namespace",
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "bla",
		},
	}

	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:create state

			stateCreate := actions[2].(kube_testing.CreateAction)
			assert.Equal(t, "create", stateCreate.GetVerb())

			state := stateCreate.GetObject().(*orch_v1.State)
			assert.Equal(t, tc.ld.Name, state.Name, "Should have name set to ld name")
			assert.Equal(t, tc.ld.Namespace, state.Namespace, "Should be created in the same namespace")

			ownerRefs := state.GetOwnerReferences()
			assert.Len(t, ownerRefs, 1, "Should have owner reference set")

			ldOwnerRef := ownerRefs[0]
			assert.True(t, *ldOwnerRef.BlockOwnerDeletion)
			assert.True(t, *ldOwnerRef.Controller)
			assert.Equal(t, tc.ld.Kind, ldOwnerRef.Kind)
			assert.Equal(t, tc.ld.Name, ldOwnerRef.Name)
			assert.Equal(t, tc.ld.UID, ldOwnerRef.UID)
		},
	}

	tc.run(t)
}

func TestCreatesStatePassesConfigMapName(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			UID:       "some-uid",
			Namespace: "parent-namespace",
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "custom-config-map",
		},
	}

	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:create state

			stateCreate := actions[2].(kube_testing.CreateAction)
			require.Equal(t, "create", stateCreate.GetVerb())

			state := stateCreate.GetObject().(*orch_v1.State)
			assert.Equal(t, "custom-config-map", state.Spec.ConfigMapName)
		},
	}

	tc.run(t)
}

func TestCreatesStateMissingConfigMapName(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			UID:       "some-uid",
			Namespace: "parent-namespace",
		},
	}

	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.EqualError(t, err, "configMapName is missing")
		},
	}

	tc.run(t)
}

func TestCreatesStateConvertsResources(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithoutTemplating(),
		},
		{
			Name: "resource-2",
			Type: "DynamoDB",
			Spec: buildLdSpecDataWithoutTemplating(),
		},
		{
			Name: "resource-3",
			Type: "type-3",
			DependsOn: []v1.LocationDescriptorDependency{
				{
					Name: "resource-1",
					Attributes: map[string]interface{}{
						"foo": "bar",
					},
				},
				{
					Name: "resource-2",
				},
			},
			Spec: buildLdSpecDataWithoutTemplating(),
		},
	})
	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:create state

			stateCreate := actions[2].(kube_testing.CreateAction)

			state := stateCreate.GetObject().(*orch_v1.State)
			assert.Equal(t, tc.ld.Name, state.Name, "Should have name set to ld name")

			assert.Len(t, state.Spec.Resources, len(ld.Spec.Resources))
			stateResources := state.Spec.Resources
			ldResources := ld.Spec.Resources

			assert.Equal(t, ldResources[0].Name, stateResources[0].Name)
			assert.Equal(t, ldResources[0].Type, stateResources[0].Type)
			assert.Equal(t, ldResources[0].Spec, stateResources[0].Spec)

			assert.Equal(t, ldResources[1].Name, stateResources[1].Name)
			assert.Equal(t, ldResources[1].Type, stateResources[1].Type)
			assert.Equal(t, []byte(`{"BackupPeriod":"1 hours"}`), stateResources[1].Defaults.Raw)

			assert.Equal(t, ldResources[2].Name, stateResources[2].Name)
			assert.Equal(t, ldResources[2].Type, stateResources[2].Type)

			assert.Len(t, stateResources[2].DependsOn, len(ldResources[2].DependsOn))
			assert.Equal(t, ldResources[2].DependsOn[0].Name, stateResources[2].DependsOn[0].Name)
			assert.Equal(t, ldResources[2].DependsOn[0].Attributes, stateResources[2].DependsOn[0].Attributes)
			assert.Equal(t, ldResources[2].DependsOn[1].Name, stateResources[2].DependsOn[1].Name)
			assert.Equal(t, ldResources[2].DependsOn[1].Attributes, stateResources[2].DependsOn[1].Attributes)
		},
	}

	tc.run(t)
}

func TestUpdatesExistingStateWithNewResources(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithoutTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithoutTemplating(),
		},
	})

	trueVar := true
	existingState := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            "test-ld",
			Namespace:       testConfigMapNamespace,
			UID:             "Some UID",
			ResourceVersion: "Some resource version",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       ld.Name,
					Kind:       ld.Kind,
					UID:        ld.UID,
				},
			},
		},
		Spec: orch_v1.StateSpec{
			// only put in the first resource
			Resources: []orch_v1.StateResource{
				{
					Name: ld.Spec.Resources[0].Name,
					Type: ld.Spec.Resources[0].Type,
					Spec: buildStateSpecDataWithoutTemplating(),
				},
			},
		},
	}

	tc := testCase{
		ld:                ld,
		orchClientObjects: []runtime.Object{existingState},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:update state

			stateUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, "update", stateUpdate.GetVerb())

			state := stateUpdate.GetObject().(*orch_v1.State)
			assert.Equal(t, existingState.ResourceVersion, state.ResourceVersion)
			assert.Equal(t, existingState.UID, state.UID)

			assert.Len(t, state.Spec.Resources, len(ld.Spec.Resources))
			stateResources := state.Spec.Resources
			ldResources := ld.Spec.Resources

			assert.Equal(t, ldResources[0].Name, stateResources[0].Name)
			assert.Equal(t, ldResources[0].Type, stateResources[0].Type)
			assert.Equal(t, ldResources[0].Spec, stateResources[0].Spec)

			assert.Equal(t, ldResources[1].Name, stateResources[1].Name)
			assert.Equal(t, ldResources[1].Type, stateResources[1].Type)
			assert.Equal(t, ldResources[1].Spec, stateResources[1].Spec)
		},
	}

	tc.run(t)
}

func TestLocationDescriptorParsingWithReleaseTemplating(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithTemplating(),
		},
	})

	tc := testCase{
		ld:          ld,
		releaseData: makeReleasesConfigMap(ld),
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:update state

			stateUpdate := actions[2].(kube_testing.CreateAction)
			assert.Equal(t, "create", stateUpdate.GetVerb())

			state := stateUpdate.GetObject().(*orch_v1.State)
			assert.NotNil(t, state)

			assert.Len(t, state.Spec.Resources, len(ld.Spec.Resources))
			stateResources := state.Spec.Resources
			ldResources := ld.Spec.Resources

			assert.Equal(t, ldResources[0].Name, stateResources[0].Name)
			assert.Equal(t, ldResources[0].Type, stateResources[0].Type)
			assert.Equal(t, stateResources[0].Spec, buildStateSpecDataWithTemplating())

			assert.Equal(t, ldResources[1].Name, stateResources[1].Name)
			assert.Equal(t, ldResources[1].Type, stateResources[1].Type)
			assert.Equal(t, stateResources[1].Spec, buildStateSpecDataWithTemplating())
		},
	}

	tc.run(t)
}

func TestLocationDescriptorWithReleaseTemplatingUsingInvalidKey(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithInvalidTemplating(),
		},
	})

	tc := testCase{
		ld:          ld,
		releaseData: makeReleasesConfigMap(ld),
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "variable not defined")

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 2)
			// 0:list,1:watch,<FAILS>
		},
	}

	tc.run(t)
}

func TestLocationDescriptorErrorPropagation(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithInvalidTemplating(),
		},
	})

	tc := testCase{
		ld:          ld,
		releaseData: makeReleasesConfigMap(ld),
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "variable not defined")

			actions := tc.formFake.Actions()
			assert.Len(t, actions, 3) // 0:list,1:watch,2:update

			assert.Equal(t, "update", actions[2].GetVerb())
			assert.Equal(t, "testNamespace", actions[2].GetNamespace())
			assert.Equal(t, form_v1.SchemeGroupVersion.WithResource(form_v1.LocationDescriptorResourcePlural), actions[2].GetResource())

			updatedLd := actions[2].(kube_testing.UpdateAction).GetObject().(*form_v1.LocationDescriptor)
			require.NotNil(t, updatedLd)
			assert.Equal(t, "test-ld", updatedLd.GetName())

			var errorCondition *cond_v1.Condition
			for _, cond := range updatedLd.Status.Conditions {
				if cond.Type == cond_v1.ConditionError {
					errorCondition = &cond
					break
				}
			}
			require.NotNil(t, errorCondition)
			assert.Equal(t, cond_v1.ConditionTrue, errorCondition.Status)
			assert.Equal(t, "variable not defined: \"INVALID.foobar\"", errorCondition.Message)
		},
	}

	tc.run(t)
}

func TestProcessLocationDescriptorWithMultipleTemplatingErrorsReturnsErrorList(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithInvalidTemplating(),
		},
		{
			Name: "resource-3",
			Type: "type-2",
			Spec: buildLdSpecDataWithInvalidTemplating(),
		},
		{
			Name: "resource-4",
			Type: "type-2",
			Spec: buildLdSpecDataWithInvalidTemplating(),
		},
	})
	tc := testCase{
		ld:          ld,
		releaseData: makeReleasesConfigMap(ld),
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.Error(t, err)
			numErrors := len(err.(*util.ErrorList).ErrorList)
			require.Equal(t, 3, numErrors) // One error per resource
			for _, e := range err.(*util.ErrorList).ErrorList {
				assert.Contains(t, e.Error(), "variable not defined")
			}

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 2)
			// 0:list,1:watch,<FAILS>
		},
	}

	tc.run(t)
}

func TestMissingConfigMapWithTemplatingKeysPresentErrorIsHandled(t *testing.T) {
	t.Parallel()

	ld := createLdWithResources([]v1.LocationDescriptorResource{
		{
			Name: "resource-1",
			Type: "type-1",
			Spec: buildLdSpecDataWithoutTemplating(),
		},
		{
			Name: "resource-2",
			Type: "type-2",
			Spec: buildLdSpecDataWithTemplating(), // Uses templating keys
		},
	})

	tc := testCase{
		ld:          ld,
		releaseData: nil, // Explicitly no release data config map set..
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "no release data was available")

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 2)
			// 0:list,1:watch,<FAILS>
		},
	}

	tc.run(t)
}

func TestErrorOnStateUpdateWhenDifferentOwner(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-ld",
		},
		// spec doesn't matter for this test
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "cm1",
			Resources: []v1.LocationDescriptorResource{
				{
					Name: "resource-1",
					Type: "type-1",
					Spec: buildLdSpecDataWithoutTemplating(),
				},
			},
		},
	}

	existingState := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:            "test-ld",
			UID:             "Some UID",
			ResourceVersion: "Some resource version",
			// no owner ref to make it not owned by the LD.
			OwnerReferences: []meta_v1.OwnerReference{},
		},
		Spec: orch_v1.StateSpec{
			// only put in the first resource
			Resources: []orch_v1.StateResource{
				{
					Name: "resource-1",
					Type: "type-1",
					Spec: buildStateSpecDataWithoutTemplating(),
				},
			},
		},
	}
	tc := testCase{
		ld:                ld,
		orchClientObjects: []runtime.Object{existingState},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.Error(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 2)
			// 0:list,1:watch
			// should not be an update
		},
	}

	tc.run(t)
}

func TestDoesNotSkipStateUpdateWhenFlaggedForDeletion(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-ld",
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "bla",
			ConfigMapNames: v1.LocationDescriptorConfigMapNames{
				Release: "releases",
			},
		},
	}

	trueVar := true
	now := meta_v1.Now()
	existingState := &orch_v1.State{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			DeletionTimestamp: &now,
			Name:              "test-ld",
			UID:               "Some UID",
			ResourceVersion:   "Some resource version",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       ld.Name,
					Kind:       ld.Kind,
					UID:        ld.UID,
				},
			},
		},
		Spec: orch_v1.StateSpec{
			Resources: []orch_v1.StateResource{
				{
					Name: "some-name",
					Type: "some-type",
					Spec: &runtime.RawExtension{
						Raw: []byte("{}"),
					},
				},
			},
		},
	}
	tc := testCase{
		ld:                ld,
		orchClientObjects: []runtime.Object{existingState},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)

			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:update
			assert.Equal(t, "update", actions[2].GetVerb())
			assert.Equal(t, orch_v1.SchemeGroupVersion.WithResource(orch_v1.StateResourcePlural), actions[2].GetResource())
		},
	}

	tc.run(t)
}

func TestHandleProcessResultHasUpdate(t *testing.T) {
	t.Parallel()

	ld := &form_v1.LocationDescriptor{}
	err := testutil.LoadIntoStructFromTestData("test_resource_update.ld.yaml", ld)
	require.NoError(t, err)
	existingState := &orch_v1.State{}
	err = testutil.LoadIntoStructFromTestData("test_resource_update.state.yaml", existingState)
	require.NoError(t, err)

	tc := testCase{
		ld:                ld,
		orchClientObjects: []runtime.Object{existingState},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {

			_, _, err := cntrlr.handleProcessResult(tc.logger, ld, existingState, false, nil)
			assert.NoError(t, err)

			// Verify actions
			actions := tc.formFake.Actions()
			assert.Len(t, actions, 3) // 0:list,1:watch,2:update

			assert.Equal(t, "update", actions[2].GetVerb())
			assert.Equal(t, "ychen-test-svc", actions[2].GetNamespace())
			assert.Equal(t, form_v1.SchemeGroupVersion.WithResource(form_v1.LocationDescriptorResourcePlural), actions[2].GetResource())
		},
	}

	tc.run(t)
}

func TestHandleProcessResultHasNoUpdate(t *testing.T) {
	t.Parallel()

	ld := &form_v1.LocationDescriptor{}
	err := testutil.LoadIntoStructFromTestData("test_resource_no_update.ld.yaml", ld)
	require.NoError(t, err)
	existingState := &orch_v1.State{}
	err = testutil.LoadIntoStructFromTestData("test_resource_no_update.state.yaml", existingState)
	require.NoError(t, err)

	tc := testCase{
		ld:                ld,
		orchClientObjects: []runtime.Object{existingState},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {

			_, _, err := cntrlr.handleProcessResult(tc.logger, ld, existingState, false, nil)
			assert.NoError(t, err)

			// Verify actions
			actions := tc.formFake.Actions()
			assert.Len(t, actions, 2) // 0:list,1:watch
			// No update
		},
	}

	tc.run(t)
}

func TestKubeComputeDefaults(t *testing.T) {
	t.Parallel()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceAPIVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			UID:       "some-uid",
			Namespace: "parent-namespace",
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "bla",
			Resources: []form_v1.LocationDescriptorResource{
				{
					Name: "test",
					Type: voyager.ResourceType("KubeCompute"),
				},
			},
		},
	}

	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			actions := tc.orchFake.Actions()
			require.Len(t, actions, 3)
			// 0:list,1:watch,2:create state

			stateCreate := actions[2].(kube_testing.CreateAction)

			state := stateCreate.GetObject().(*orch_v1.State)
			assert.Equal(t, tc.ld.Name, state.Name, "Should have name set to ld name")

			assert.Len(t, state.Spec.Resources, len(ld.Spec.Resources))
			stateResources := state.Spec.Resources
			ldResources := ld.Spec.Resources

			assert.Equal(t, ldResources[0].Name, stateResources[0].Name)
			assert.Equal(t, ldResources[0].Type, stateResources[0].Type)
			assert.Equal(t, ldResources[0].Spec, stateResources[0].Spec)

			expected := `{"Container":{"ImagePullPolicy":"IfNotPresent","LivenessProbe":{"FailureThreshold":3,"HTTPGet":{"Path":"/healthcheck","Scheme":"HTTP"},"PeriodSeconds":10,"SuccessThreshold":1,"TimeoutSeconds":1},"ReadinessProbe":{"FailureThreshold":3,"HTTPGet":{"Path":"/healthcheck","Scheme":"HTTP"},"PeriodSeconds":10,"SuccessThreshold":1,"TimeoutSeconds":1},"Resources":{"Limits":{"cpu":"1","memory":"1Gi"},"Requests":{"cpu":"150m","memory":"750Mi"}}},"Port":{"Protocol":"TCP"},"Scaling":{"MaxReplicas":5,"Metrics":[{"Resource":{"Name":"cpu","TargetAverageUtilization":80},"Type":"Resource"}],"MinReplicas":1}}`
			assert.Equal(t, []byte(expected), state.Spec.Resources[0].Defaults.Raw)
		},
	}

	tc.run(t)
}

func TestByReleaseConfigMapNameIndexConstructValidIndexKey(t *testing.T) {
	t.Parallel()

	ldNamespace := "somenamespace"
	releaseConfigMapKey := "releases"

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			Namespace: ldNamespace,
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "bla",
			ConfigMapNames: v1.LocationDescriptorConfigMapNames{
				Release: releaseConfigMapKey,
			},
		},
	}

	res, err := ByReleaseConfigMapNameIndex(ld)
	assert.NoError(t, err)
	assert.Equal(t, ByConfigMapNameIndexKey(ldNamespace, releaseConfigMapKey), res[0])
}

func TestDeletedLocationDescriptorIsSkipped(t *testing.T) {
	t.Parallel()

	ldNamespace := "somenamespace"
	now := meta_v1.Now()

	ld := &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              "test-ld",
			Namespace:         ldNamespace,
			DeletionTimestamp: &now,
		},
		Spec: v1.LocationDescriptorSpec{},
	}

	tc := testCase{
		ld: ld,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, err := cntrlr.Process(ctx)
			require.NoError(t, err)

			orchActions := tc.orchFake.Actions()
			require.Len(t, orchActions, 2) // list + watch only
			formActions := tc.formFake.Actions()
			assert.Len(t, formActions, 2) // list + watch only
		},
	}

	tc.run(t)
}

func buildLdSpecDataWithoutTemplating() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: createJsonBytes(`
			{
				"sd": {
					"links": {
						"binary": {
							"name": "docker.example.com/micros/node-refapp",
							"tag": "tag-1234",
							"type": "docker"
						}
					},
					"healthcheck": {
						"uri": "/healthcheck",
						"source": {
							"url": "ssh://git@stash.atlassian.com:7997/micros/node-refapp.git"
						}
					},
					"notifications": {
						"email": "an_owner@example.com"
					}
				}
			}
		`),
	}
}

func buildLdSpecDataWithTemplating() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: createJsonBytes(`
			{
				"sd": {
					"links": {
						"binary": {
							"name": "docker.example.com/micros/node-refapp",
							"tag": "${release:foobar}--${release:other}",
							"type": "docker"
						}
					},
					"healthcheck": {
						"uri": "${release:deep.foobar}",
						"source": {
							"url": "ssh://git@stash.atlassian.com:7997/micros/node-refapp.git"
						}
					},
					"notifications": {
						"email": "an_owner@example.com"
					}
				}
			}
		`),
	}
}

func buildLdSpecDataWithInvalidTemplating() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: createJsonBytes(`
			{
				"sd": {
					"links": {
						"binary": {
							"name": "docker.example.com/micros/node-refapp",
							"tag": "${release:foobar}",
							"type": "docker"
						}
					},
					"healthcheck": {
						"uri": "${release:INVALID.foobar}",
						"source": {
							"url": "ssh://git@stash.atlassian.com:7997/micros/node-refapp.git"
						}
					},
					"notifications": {
						"email": "an_owner@example.com"
					}
				}
			}
		`),
	}
}

func buildStateSpecDataWithoutTemplating() *runtime.RawExtension {
	return buildLdSpecDataWithoutTemplating()
}

func buildStateSpecDataWithTemplating() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: createJsonBytes(`
			{
				"sd": {
					"links": {
						"binary": {
							"name": "docker.example.com/micros/node-refapp",
							"tag": "123--456",
							"type": "docker"
						}
					},
					"healthcheck": {
						"uri": "something else",
						"source": {
							"url": "ssh://git@stash.atlassian.com:7997/micros/node-refapp.git"
						}
					},
					"notifications": {
						"email": "an_owner@example.com"
					}
				}
			}
		`),
	}
}

func createJsonBytes(str string) []byte {
	var obj = make(map[string]interface{})
	err := json.Unmarshal([]byte(str), &obj)
	if err == nil {
		res, _ := json.Marshal(obj)
		return res
	}
	panic(fmt.Sprintf("your test data is wrong! Error: %s", err))
}

func makeReleasesConfigMap(ld *form_v1.LocationDescriptor) *core_v1.ConfigMap {
	releaseData := make(map[string]string, 1)
	toSerialize := map[string]interface{}{
		"foobar": 123,
		"other":  456,
		"deep": map[string]interface{}{
			"foobar": "something else",
		},
	}

	bytes, err := yaml.Marshal(toSerialize)
	if err != nil {
		panic(err)
	}
	releaseData[releaseConfigMapDataKey] = fmt.Sprintf("%s", bytes)

	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.ConfigMapKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      ld.Spec.ConfigMapNames.Release,
			Namespace: ld.Namespace,
		},
		Data: releaseData,
	}
}

func createLdWithResources(ldResources []v1.LocationDescriptorResource) *form_v1.LocationDescriptor {
	return &v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       v1.LocationDescriptorResourceKind,
			APIVersion: v1.LocationDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ld",
			Namespace: testConfigMapNamespace,
		},
		Spec: v1.LocationDescriptorSpec{
			ConfigMapName: "cm1",
			Resources:     ldResources,
			ConfigMapNames: v1.LocationDescriptorConfigMapNames{
				Release: "foobarName",
			},
		},
	}
}

type testCase struct {
	formClientObjects []runtime.Object
	orchClientObjects []runtime.Object
	mainClientObjects []runtime.Object

	ld          *v1.LocationDescriptor
	releaseData *core_v1.ConfigMap

	test func(*testing.T, *Controller, *ctrl.ProcessContext, *testCase)

	// These are written by the run so the test function can access them
	logger   *zap.Logger
	formFake *kube_testing.Fake
	orchFake *kube_testing.Fake
}

func (tc *testCase) run(t *testing.T) {
	objects := append(tc.formClientObjects, tc.ld)
	ldClient := formclient_fake.NewSimpleClientset(objects...)
	tc.formFake = &ldClient.Fake

	if tc.releaseData != nil {
		tc.mainClientObjects = append(tc.mainClientObjects, tc.releaseData)
	}
	mainClient := k8s_fake.NewSimpleClientset(tc.mainClientObjects...)

	stateClient := orchclient_fake.NewSimpleClientset(tc.orchClientObjects...)
	tc.orchFake = &stateClient.Fake

	logger := zaptest.NewLogger(t)
	tc.logger = logger

	config := &ctrl.Config{
		Logger:       logger,
		ResyncPeriod: time.Second * 60,
	}

	ldInformer := formInf.LocationDescriptorInformer(ldClient, meta_v1.NamespaceAll, config.ResyncPeriod)
	stateInformer := orchInf.StateInformer(stateClient, meta_v1.NamespaceAll, config.ResyncPeriod)
	cmInformer := core_v1inf.NewConfigMapInformer(mainClient, testConfigMapNamespace, config.ResyncPeriod, cache.Indexers{})

	informers := []cache.SharedIndexInformer{ldInformer, stateInformer, cmInformer}

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	// Start all informers then wait on them
	for _, inf := range informers {
		stage.StartWithChannel(inf.Run)
	}
	for _, inf := range informers {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	objectUpdater := formUpdater.StateUpdater(stateInformer.GetIndexer(), specCheck, stateClient)

	// Metrics
	ldTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "test",
			Name:      "ld_transitions_total",
			Help:      "Test Counter",
		},
		[]string{"namespace", "name", "type", "reason"},
	)

	cntrlr := &Controller{
		Logger: logger,
		Clock:  clock.NewFakeClock(time.Unix(0, 0)),

		LDInformer:        ldInformer,
		StateInformer:     stateInformer,
		ConfigMapInformer: cmInformer,

		LDClient: ldClient.FormationV1(),

		LDTransitionsCounter: ldTransitionsCounter,

		StateObjectUpdater: objectUpdater,

		// right now we don't set the workQueue, we call Process directly
	}

	pctx := &ctrl.ProcessContext{
		Logger: logger,
		Object: tc.ld,
	}

	tc.test(t, cntrlr, pctx, tc)

}
