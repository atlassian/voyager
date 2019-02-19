package composition

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	compclient_fake "github.com/atlassian/voyager/pkg/composition/client/fake"
	compUpdater "github.com/atlassian/voyager/pkg/composition/updater"
	formclient_fake "github.com/atlassian/voyager/pkg/formation/client/fake"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	"github.com/atlassian/voyager/pkg/k8s"
	k8s_testing "github.com/atlassian/voyager/pkg/k8s/testing"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/options"
	apisynchronization "github.com/atlassian/voyager/pkg/synchronization/api"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	k8s_fake "k8s.io/client-go/kubernetes/fake"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

const (
	fixtureServiceDescriptorInputSuffix  = ".sd.input.yaml"
	fixtureLocationDescriptorsGlobSuffix = ".ld.*.yaml"
	fixtureServiceDescriptorOutputSuffix = ".sd.output.yaml"

	fixtureGlob = "*" + fixtureServiceDescriptorInputSuffix
)

func testHandleProcessResult(t *testing.T, filePrefix string) {
	sd := &comp_v1.ServiceDescriptor{}
	err := testutil.LoadIntoStructFromTestData(filePrefix+fixtureServiceDescriptorInputSuffix, sd)
	require.NoError(t, err)

	ldFiles, err := filepath.Glob(filepath.Join(testutil.FixturesDir, filePrefix+fixtureLocationDescriptorsGlobSuffix))
	require.NoError(t, err)
	results := make([]formationObjectResult, 0, len(ldFiles))

	for _, ldFile := range ldFiles {
		// Bunch of string splitting
		_, filename := filepath.Split(ldFile)

		// Load a list of location descriptors actually..
		ld := &form_v1.LocationDescriptor{}
		err := testutil.LoadIntoStructFromTestData(filename, ld)
		require.NoError(t, err)

		serviceName, serviceLabel := deconstructNamespaceName(ld.Namespace)

		results = append(results, formationObjectResult{
			ld: ld,
			namespace: &core_v1.Namespace{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.NamespaceKind,
					APIVersion: core_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "test-sd",
					Labels: map[string]string{
						voyager.ServiceNameLabel:  serviceName,
						voyager.ServiceLabelLabel: string(serviceLabel),
					},
				},
			},
		})
	}

	testClock := clock.NewFakeClock(time.Date(2015, 10, 15, 9, 30, 0, 0, time.FixedZone("New_York", int(-4*time.Hour/time.Second))))

	tc := testCase{
		sd:    sd,
		clock: testClock,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			_, _, err := cntrlr.handleProcessResult(ctx.Logger, sd.Name, sd, results, false, false, nil)
			assert.NoError(t, err)

			// Compare the outputs
			fileName := filePrefix + fixtureServiceDescriptorOutputSuffix
			sdExpected := &comp_v1.ServiceDescriptor{}
			err = testutil.LoadIntoStructFromTestData(fileName, sdExpected)
			require.NoError(t, err)

			outputSd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			testutil.ObjectCompareContext(t, testutil.FileName(fileName), outputSd, sdExpected)
		},
	}

	tc.run(t)
}

func TestCompositionWithTestData(t *testing.T) {
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
		sdFileName := strings.Split(filename, ".")
		resultFilePrefix := strings.Join(sdFileName[:len(sdFileName)-3], ".")

		t.Run(resultFilePrefix, func(t *testing.T) {
			testHandleProcessResult(t, resultFilePrefix)
		})
	}
}

func TestCreatesNamespaceNoLabel(t *testing.T) {
	t.Parallel()

	tc := testCase{
		sd: &comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       "test-sd",
				UID:        "the-sd-uid",
				Finalizers: []string{FinalizerServiceDescriptorComposition},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					locationNoLabel(),
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			cntrlr.Process(ctx)

			ns, ok := findCreatedNamespace(tc.mainFake.Actions())
			require.True(t, ok)

			assert.Equal(t, tc.sd.Name, ns.Name, "Should have name set to sd name")

			expectedLabels := map[string]string{
				voyager.ServiceNameLabel:  tc.sd.Name,
				voyager.ServiceLabelLabel: "",
			}
			assert.Equal(t, expectedLabels, ns.GetLabels())

			ownerRefs := ns.GetOwnerReferences()
			assert.Len(t, ownerRefs, 1, "Should have owner reference set")

			sdOwnerRef := ownerRefs[0]
			assert.True(t, *sdOwnerRef.BlockOwnerDeletion)
			assert.True(t, *sdOwnerRef.Controller)
			assert.Equal(t, tc.sd.Kind, sdOwnerRef.Kind)
			assert.Equal(t, tc.sd.Name, sdOwnerRef.Name)
			assert.Equal(t, tc.sd.UID, sdOwnerRef.UID)
		},
	}

	tc.run(t)
}

func TestCreatesNamespaceWithLabel(t *testing.T) {
	t.Parallel()

	tc := testCase{
		sd: &comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       "test-sd",
				UID:        "the-sd-uid",
				Finalizers: []string{FinalizerServiceDescriptorComposition},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					locationWithLabel(),
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			cntrlr.Process(ctx)

			ns, ok := findCreatedNamespace(tc.mainFake.Actions())
			require.True(t, ok)

			assert.Equal(t, fmt.Sprintf("%s--%s", tc.sd.Name, tc.sd.Spec.Locations[0].Label),
				ns.Name, "Should have name set to sd name combined with location label")

			expectedLabels := map[string]string{
				voyager.ServiceNameLabel:  tc.sd.Name,
				voyager.ServiceLabelLabel: string(tc.sd.Spec.Locations[0].Label),
			}
			assert.Equal(t, expectedLabels, ns.GetLabels())

			ownerRefs := ns.GetOwnerReferences()
			assert.Len(t, ownerRefs, 1, "Should have owner reference set")

			sdOwnerRef := ownerRefs[0]
			assert.True(t, *sdOwnerRef.BlockOwnerDeletion)
			assert.True(t, *sdOwnerRef.Controller)
			assert.Equal(t, tc.sd.Kind, sdOwnerRef.Kind)
			assert.Equal(t, tc.sd.Name, sdOwnerRef.Name)
			assert.Equal(t, tc.sd.UID, sdOwnerRef.UID)
		},
	}

	tc.run(t)
}

func TestCreatesLocationDescriptorNoLabel(t *testing.T) {
	t.Parallel()

	tc := testCase{
		sd: &comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       comp_v1.ServiceDescriptorResourceKind,
				APIVersion: comp_v1.ServiceDescriptorResourceVersion,
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       "test-sd",
				Finalizers: []string{FinalizerServiceDescriptorComposition},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					locationNoLabel(),
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			ld, ok := findCreatedLocationDescriptor(tc.formFake.Actions())
			require.True(t, ok)

			assert.Equal(t, tc.sd.Name, ld.Name, "Should have name set to sd name")
		},
	}

	tc.run(t)
}

func TestCreatesLocationDescriptorWithTransformedResources(t *testing.T) {
	t.Parallel()

	location := locationNoLabel()
	sdWithResources := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: comp_v1.ServiceDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				location,
			},
			// it doesn't matter what we put here
			// because it gets the output of the transformer
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
		},
	}
	tc := testCase{
		sd: sdWithResources,
		transformedResources: []comp_v1.ServiceDescriptorResource{
			{
				Name: "first-resource",
				Type: "first-type",
			},
			{
				Name: "second-resource",
				Type: "second-type",
			},
			{
				Name: "third-resource",
				Type: "third-type",
				DependsOn: []comp_v1.ServiceDescriptorResourceDependency{
					{
						Name: "second-resource",
						Attributes: map[string]interface{}{
							"Bar":   "blah",
							"Other": "foo",
						},
					},
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			ld, ok := findCreatedLocationDescriptor(tc.formFake.Actions())
			require.True(t, ok)

			assert.Equal(t, tc.sd.Name, ld.Name, "Should have name set to sd name")

			assert.Equal(t, apisynchronization.DefaultServiceMetadataConfigMapName, ld.Spec.ConfigMapName)

			assert.Len(t, ld.Spec.Resources, len(tc.transformedResources))

			assert.Equal(t, tc.transformedResources[0].Name, ld.Spec.Resources[0].Name)
			assert.Equal(t, tc.transformedResources[0].Type, ld.Spec.Resources[0].Type)

			assert.Equal(t, tc.transformedResources[1].Name, ld.Spec.Resources[1].Name)
			assert.Equal(t, tc.transformedResources[1].Type, ld.Spec.Resources[1].Type)

			assert.Equal(t, tc.transformedResources[2].Name, ld.Spec.Resources[2].Name)
			assert.Equal(t, tc.transformedResources[2].Type, ld.Spec.Resources[2].Type)
			assert.Len(t, tc.transformedResources[2].DependsOn, len(tc.transformedResources[2].DependsOn))
			assert.Equal(t, tc.transformedResources[2].DependsOn[0].Name, ld.Spec.Resources[2].DependsOn[0].Name)
			assert.Equal(t, tc.transformedResources[2].DependsOn[0].Attributes, ld.Spec.Resources[2].DependsOn[0].Attributes)
		},
	}

	tc.run(t)
}

func TestCreatesLocationDescriptorWithLabel(t *testing.T) {
	t.Parallel()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: comp_v1.ServiceDescriptorResourceVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationWithLabel(),
			},
		},
	}

	expectedName := fmt.Sprintf("%s--%s", sd.Name, sd.Spec.Locations[0].Label)

	tc := testCase{
		sd: sd,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			ld, ok := findCreatedLocationDescriptor(tc.formFake.Actions())
			require.True(t, ok)

			assert.Equal(t, expectedName, ld.Name, "Should have name set to sd name")
		},
	}

	tc.run(t)
}

func TestUpdatesLocationDescriptorNoLabel(t *testing.T) {
	t.Parallel()

	existingSD := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			UID:        "the-sd-uid",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			// doesn't matter, since we get it from the transformer
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{},
		},
	}
	trueVar := true
	existingLocationDescriptor := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd",
			Namespace: "test-sd",
			UID:       "some-uid",
		},
		Spec: form_v1.LocationDescriptorSpec{
			ConfigMapName: "cm1",
			Resources: []form_v1.LocationDescriptorResource{
				{
					Name: "old-resource",
					Type: "some-type",
				},
			},
		},
	}
	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       existingSD.Name,
					Kind:       existingSD.Kind,
					UID:        existingSD.UID,
				},
			},
		},
	}

	tc := testCase{
		formClientObjects: []runtime.Object{existingLocationDescriptor},
		mainClientObjects: []runtime.Object{existingNamespace},
		sd:                existingSD,
		transformedResources: []comp_v1.ServiceDescriptorResource{
			{
				Name: "first-resource",
				Type: "first-type",
			},
			{
				Name: "second-resource",
				Type: "second-type",
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			ld, ok := findUpdatedLocationDescriptor(tc.formFake.Actions())
			require.True(t, ok)

			assert.Equal(t, tc.sd.Name, ld.Name)
			assert.Equal(t, existingLocationDescriptor.GetUID(), ld.GetUID())

			// make sure it has the new resources we setup as expected
			require.Len(t, ld.Spec.Resources, len(tc.transformedResources))
			assert.Equal(t, tc.transformedResources[0].Name, ld.Spec.Resources[0].Name)
			assert.Equal(t, tc.transformedResources[0].Type, ld.Spec.Resources[0].Type)

			assert.Equal(t, tc.transformedResources[1].Name, ld.Spec.Resources[1].Name)
			assert.Equal(t, tc.transformedResources[1].Type, ld.Spec.Resources[1].Type)
		},
	}

	tc.run(t)
}

func TestDoesNotSkipLocationDescriptorUpdateWhenLocationDescriptorBeingDeleted(t *testing.T) {
	t.Parallel()

	now := meta_v1.Now()
	existingSD := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       comp_v1.ServiceDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
		},
	}
	existingLocationDescriptor := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              "test-sd",
			Namespace:         "test-sd",
			UID:               "some-uid",
			DeletionTimestamp: &now,
		},
	}
	trueVar := true
	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       existingSD.Name,
					Kind:       existingSD.Kind,
					UID:        existingSD.UID,
				},
			},
		},
	}

	tc := testCase{
		formClientObjects: []runtime.Object{existingLocationDescriptor},
		mainClientObjects: []runtime.Object{existingNamespace},
		sd:                existingSD,
		transformedResources: []comp_v1.ServiceDescriptorResource{
			{
				Name: "first-resource",
				Type: "first-type",
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			_, ok := findCreatedLocationDescriptor(tc.formFake.Actions())
			assert.False(t, ok)
			_, ok = findUpdatedLocationDescriptor(tc.formFake.Actions())
			assert.True(t, ok)
		},
	}

	tc.run(t)
}

func TestLocationDescriptorErrorsWhenDifferentOwnerReference(t *testing.T) {
	t.Parallel()

	existingLocationDescriptor := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd",
			Namespace: "test-sd",
			UID:       "some-uid",
		},
	}

	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			// non matching owner reference
			OwnerReferences: []meta_v1.OwnerReference{},
		},
	}

	tc := testCase{
		formClientObjects: []runtime.Object{existingLocationDescriptor},
		mainClientObjects: []runtime.Object{existingNamespace},
		sd: &comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       comp_v1.ServiceDescriptorResourceKind,
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       "test-sd",
				Finalizers: []string{FinalizerServiceDescriptorComposition},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					locationNoLabel(),
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)

			assert.Error(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			_, ok := findCreatedLocationDescriptor(tc.formFake.Actions())
			require.False(t, ok)
			_, ok = findUpdatedLocationDescriptor(tc.formFake.Actions())
			require.False(t, ok)
		},
	}

	tc.run(t)
}

func TestSkipsLocationWhenControllerHasNamespace(t *testing.T) {
	t.Parallel()

	tc := testCase{
		sd: &comp_v1.ServiceDescriptor{
			TypeMeta: meta_v1.TypeMeta{},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "test-sd",
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					locationNoLabel(),
				},
			},
		},
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			cntrlr.namespace = "some-random-ns"
			external, retriable, err := cntrlr.Process(ctx)

			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			_, found := findCreatedNamespace(tc.mainFake.Actions())
			require.False(t, found)
		},
	}
	tc.run(t)
}

func TestServiceDescriptorUpdatedIfStatusChanges(t *testing.T) {
	t.Parallel()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
	}

	tc := testCase{
		sd: sd,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			sd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			require.Len(t, sd.Status.Conditions, 3)
			require.Len(t, sd.Status.LocationStatuses, 1)
			assert.Equal(t, sd.Spec.Locations[0].VoyagerLocation(), sd.Status.LocationStatuses[0].Location)
			require.Len(t, sd.Status.LocationStatuses[0].Conditions, 3)
		},
	}
	tc.run(t)
}

func TestServiceDescriptorFinalizerAdded(t *testing.T) {
	t.Parallel()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
	}

	tc := testCase{
		sd: sd,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			sd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			require.True(t, hasServiceDescriptorFinalizer(sd))
		},
	}
	tc.run(t)
}

func TestDeleteServiceDescriptorFinalizerRemoved(t *testing.T) {
	t.Parallel()

	ts, _ := time.Parse(time.RFC3339, "2018-08-01T01:10:00Z")
	deletionTimestamp := meta_v1.NewTime(ts)
	// emulate extra finalizer added by some third party, should be left untouched
	thirdPartyFinalizer := "thirdParty/Finalizer"
	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              "test-sd",
			DeletionTimestamp: &deletionTimestamp,
			Finalizers: []string{
				FinalizerServiceDescriptorComposition,
				thirdPartyFinalizer,
			},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
	}

	tc := testCase{
		sd: sd,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			sd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			require.False(t, hasServiceDescriptorFinalizer(sd))
			require.True(t, resources.HasFinalizer(sd, thirdPartyFinalizer))
			require.Len(t, sd.Status.Conditions, 3)
			require.Len(t, sd.Status.LocationStatuses, 0)
		},
	}
	tc.run(t)
}

func TestServiceDescriptorNotUpdatedIfStatusNotChanged(t *testing.T) {
	t.Parallel()
	voyagerLocation := locationNoLabel().VoyagerLocation()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
		// Yeah I'm going to fake this entire thing
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: meta_v1.Now(), // timestamp doesn't matter
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
			LocationStatuses: []comp_v1.LocationStatus{
				comp_v1.LocationStatus{
					DescriptorName:      "test-sd",
					DescriptorNamespace: "test-sd",
					Location:            voyagerLocation,
					Conditions: []cond_v1.Condition{
						{
							Type:   cond_v1.ConditionError,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionInProgress,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionReady,
							Status: cond_v1.ConditionFalse,
						},
					},
				},
			},
		},
	}

	tc := testCase{
		sd: sd,
		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			_, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.False(t, ok)
		},
	}
	tc.run(t)
}

func TestServiceDescriptorCopiesLdStatus(t *testing.T) {
	t.Parallel()

	ts1 := meta_v1.Time{time.Now().Add(time.Second)}
	ts2 := meta_v1.Time{time.Now().Add(2 * time.Second)}
	ts3 := meta_v1.Time{time.Now().Add(3 * time.Second)}
	voyagerLocation := locationNoLabel().VoyagerLocation()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Finalizers: []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
			LocationStatuses: []comp_v1.LocationStatus{
				{
					DescriptorName:      "test-sd",
					DescriptorNamespace: "test-sd",
					Location:            voyagerLocation,
					Conditions: []cond_v1.Condition{
						{
							Type:   cond_v1.ConditionError,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionInProgress,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionReady,
							Status: cond_v1.ConditionFalse,
						},
					},
				},
			},
		},
	}

	ld := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd",
			Namespace: "test-sd",
			UID:       "some-uid",
			// just setting any owner reference value means it shouldn't match
		},
		Spec: form_v1.LocationDescriptorSpec{
			ConfigMapName: "service-metadata",
			ConfigMapNames: form_v1.LocationDescriptorConfigMapNames{
				Release: "service-release",
			},
		},
		Status: form_v1.LocationDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: ts1,
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionTrue,
					Message:            "oh no",
					Reason:             "TerminalError",
				},
				{
					LastTransitionTime: ts2,
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
				},
				{
					LastTransitionTime: ts3,
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
		},
	}
	unreferencedLd := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-sd--mylabel",
			Namespace: "test-sd--mylabel",
			UID:       "some-uid",
			// just setting any owner reference value means it shouldn't match
		},
		Spec: form_v1.LocationDescriptorSpec{
			ConfigMapName: "service-metadata",
			ConfigMapNames: form_v1.LocationDescriptorConfigMapNames{
				Release: "service-release",
			},
		},
		Status: form_v1.LocationDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: ts1,
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionTrue,
					Message:            "oh no",
					Reason:             "TerminalError",
				},
				{
					LastTransitionTime: ts2,
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
				},
				{
					LastTransitionTime: ts3,
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
		},
	}
	trueVar := true
	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       sd.Name,
					Kind:       sd.Kind,
					UID:        sd.UID,
				},
			},
			Labels: map[string]string{
				voyager.ServiceNameLabel:  "test-sd",
				voyager.ServiceLabelLabel: "",
			},
		},
	}
	unreferencedNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd--mylabel",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       sd.Name,
					Kind:       sd.Kind,
					UID:        sd.UID,
				},
			},
			Labels: map[string]string{
				voyager.ServiceNameLabel:  "test-sd",
				voyager.ServiceLabelLabel: "mylabel",
			},
		},
	}

	tc := testCase{
		sd:                sd,
		formClientObjects: []runtime.Object{ld, unreferencedLd},
		mainClientObjects: []runtime.Object{existingNamespace, unreferencedNamespace},

		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			sd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			require.Len(t, sd.Status.Conditions, 3)
			require.Len(t, sd.Status.LocationStatuses, 2)
			baseLocation := sd.Spec.Locations[0].VoyagerLocation()
			assert.Equal(t, baseLocation, sd.Status.LocationStatuses[0].Location)
			assert.Equal(t, baseLocation.ClusterLocation().Location("mylabel"), sd.Status.LocationStatuses[1].Location)

			for _, locStatus := range sd.Status.LocationStatuses {
				ldConditions := locStatus.Conditions
				require.Len(t, ldConditions, 3)

				_, errCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionError)
				assert.Equal(t, &cond_v1.Condition{
					LastTransitionTime: ts1,
					Message:            "oh no",
					Reason:             "TerminalError",
					Status:             cond_v1.ConditionTrue,
					Type:               cond_v1.ConditionError,
				}, errCond)
				_, inProgressCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionInProgress)
				assert.Equal(t, &cond_v1.Condition{
					LastTransitionTime: ts2,
					Status:             cond_v1.ConditionTrue,
					Type:               cond_v1.ConditionInProgress,
				}, inProgressCond)
				_, readyCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionReady)
				assert.Equal(t, &cond_v1.Condition{
					LastTransitionTime: ts3,
					Status:             cond_v1.ConditionFalse,
					Type:               cond_v1.ConditionReady,
				}, readyCond)
			}
		},
	}
	tc.run(t)
}

func TestServiceDescriptorCopiesLdStatusWhenDeleting(t *testing.T) {
	t.Parallel()

	ts1 := meta_v1.Time{time.Now().Add(time.Second)}
	ts2 := meta_v1.Time{time.Now().Add(2 * time.Second)}
	ts3 := meta_v1.Time{time.Now().Add(3 * time.Second)}
	voyagerLocation := locationNoLabel().VoyagerLocation()

	sd := &comp_v1.ServiceDescriptor{
		TypeMeta: meta_v1.TypeMeta{},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              "test-sd",
			DeletionTimestamp: &ts1,
			Finalizers:        []string{FinalizerServiceDescriptorComposition},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Locations: []comp_v1.ServiceDescriptorLocation{
				locationNoLabel(),
			},
			ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
				simpleResourceGroup(),
			},
		},
		Status: comp_v1.ServiceDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionFalse,
				},
				{
					LastTransitionTime: meta_v1.Now(),
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
			LocationStatuses: []comp_v1.LocationStatus{
				{
					DescriptorName:      "test-sd",
					DescriptorNamespace: "test-sd",
					Location:            voyagerLocation,
					Conditions: []cond_v1.Condition{
						{
							Type:   cond_v1.ConditionError,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionInProgress,
							Status: cond_v1.ConditionFalse,
						},
						{
							Type:   cond_v1.ConditionReady,
							Status: cond_v1.ConditionFalse,
						},
					},
				},
			},
		},
	}

	ld := &form_v1.LocationDescriptor{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       form_v1.LocationDescriptorResourceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:       "test-sd",
			Namespace:  "test-sd",
			UID:        "some-uid",
			Finalizers: []string{"someFinalizer"},
		},
		// Whatever, don't care about Spec, we just need an existing LD with a matching name
		Status: form_v1.LocationDescriptorStatus{
			Conditions: []cond_v1.Condition{
				{
					LastTransitionTime: ts1,
					Type:               cond_v1.ConditionError,
					Status:             cond_v1.ConditionTrue,
					Message:            "oh no",
					Reason:             "TerminalError",
				},
				{
					LastTransitionTime: ts2,
					Type:               cond_v1.ConditionInProgress,
					Status:             cond_v1.ConditionTrue,
				},
				{
					LastTransitionTime: ts3,
					Type:               cond_v1.ConditionReady,
					Status:             cond_v1.ConditionFalse,
				},
			},
		},
	}
	trueVar := true
	existingNamespace := &core_v1.Namespace{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.NamespaceKind,
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			OwnerReferences: []meta_v1.OwnerReference{
				{
					Controller: &trueVar,
					Name:       sd.Name,
					Kind:       sd.Kind,
					UID:        sd.UID,
				},
			},
		},
	}

	tc := testCase{
		sd:                sd,
		formClientObjects: []runtime.Object{ld},
		mainClientObjects: []runtime.Object{existingNamespace},

		test: func(t *testing.T, cntrlr *Controller, ctx *ctrl.ProcessContext, tc *testCase) {
			// Make LD deletion a no-op rather than deleting it from cache straight away, to emulate it being asynchronously deleted
			tc.formFake.PrependReactor("delete", "locationdescriptors", func(action kube_testing.Action) (bool, runtime.Object, error) {
				return true, nil, nil
			})

			external, retriable, err := cntrlr.Process(ctx)
			require.NoError(t, err)
			assert.False(t, external, "error should not be an external error")
			assert.False(t, retriable, "error should not be an external error")

			// Descriptor should have updated with a bunch of statuses
			sd, ok := findUpdatedServiceDescriptor(tc.compFake.Actions())
			require.True(t, ok)

			require.Len(t, sd.Status.Conditions, 3)
			require.Len(t, sd.Status.LocationStatuses, 1)
			assert.Equal(t, sd.Spec.Locations[0].VoyagerLocation(), sd.Status.LocationStatuses[0].Location)

			ldConditions := sd.Status.LocationStatuses[0].Conditions
			require.Len(t, ldConditions, 3)

			_, errCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionError)
			assert.Equal(t, &cond_v1.Condition{
				LastTransitionTime: ts1,
				Message:            "oh no",
				Reason:             "TerminalError",
				Status:             cond_v1.ConditionTrue,
				Type:               cond_v1.ConditionError,
			}, errCond)
			_, inProgressCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionInProgress)
			assert.Equal(t, &cond_v1.Condition{
				LastTransitionTime: ts2,
				Status:             cond_v1.ConditionTrue,
				Type:               cond_v1.ConditionInProgress,
			}, inProgressCond)
			_, readyCond := cond_v1.FindCondition(ldConditions, cond_v1.ConditionReady)
			assert.Equal(t, &cond_v1.Condition{
				LastTransitionTime: ts3,
				Status:             cond_v1.ConditionFalse,
				Type:               cond_v1.ConditionReady,
			}, readyCond)

			// SD should still have a finalizers
			require.True(t, hasServiceDescriptorFinalizer(sd))
		},
	}
	tc.run(t)
}

func simpleResourceGroup() comp_v1.ServiceDescriptorResourceGroup {
	return comp_v1.ServiceDescriptorResourceGroup{
		Locations: []comp_v1.ServiceDescriptorLocationName{"some-location"},
		Name:      "some-resource-group",
		Resources: []comp_v1.ServiceDescriptorResource{
			comp_v1.ServiceDescriptorResource{
				Name: "resource1",
				Type: "EC2Compute",
			},
		},
	}
}

func locationNoLabel() comp_v1.ServiceDescriptorLocation {
	return comp_v1.ServiceDescriptorLocation{
		Name:    "some-location",
		Account: "12345",
		Region:  "ap-eastwest-1",
		EnvType: "test",
	}
}

func locationWithLabel() comp_v1.ServiceDescriptorLocation {
	location := locationNoLabel()
	location.Label = "my-expected-label"
	return location
}

type testCase struct {
	logger *zap.Logger
	clock  *clock.FakeClock

	mainClientObjects []runtime.Object
	formClientObjects []runtime.Object
	compClientObjects []runtime.Object

	sd                   *comp_v1.ServiceDescriptor
	transformedResources []comp_v1.ServiceDescriptorResource

	test func(*testing.T, *Controller, *ctrl.ProcessContext, *testCase)

	mainFake *kube_testing.Fake
	compFake *kube_testing.Fake
	formFake *kube_testing.Fake
}

func (tc *testCase) run(t *testing.T) {
	mainClientObjects := tc.mainClientObjects
	mainClient := k8s_fake.NewSimpleClientset(mainClientObjects...)
	tc.mainFake = &mainClient.Fake

	formationClient := formclient_fake.NewSimpleClientset(tc.formClientObjects...)
	tc.formFake = &formationClient.Fake

	compClientObjects := append(tc.compClientObjects, tc.sd)
	compositionClient := compclient_fake.NewSimpleClientset(compClientObjects...)
	tc.compFake = &compositionClient.Fake

	logger := zaptest.NewLogger(t)
	config := &ctrl.Config{
		Logger:       logger,
		ResyncPeriod: time.Second * 60,
		MainClient:   mainClient,
	}

	nsInformer := core_v1inf.NewNamespaceInformer(mainClient, config.ResyncPeriod, cache.Indexers{
		nsServiceNameIndex: nsServiceNameIndexFunc,
	})
	ldInformer := formInf.LocationDescriptorInformer(formationClient, meta_v1.NamespaceAll, config.ResyncPeriod)
	err := ldInformer.AddIndexers(cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	require.NoError(t, err)
	informers := []cache.SharedIndexInformer{ldInformer, nsInformer}

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	// Object Updaters
	// Copy and pasted from the constructor...
	ldUpdater := compUpdater.LocationDescriptorUpdater(ldInformer.GetIndexer(), specCheck, formationClient)
	namespaceUpdater := updater.NamespaceUpdater(nsInformer.GetIndexer(), specCheck, config.MainClient)

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

	fakeSd := fakeSdTransformer{}

	objectInfos := make([]FormationObjectInfo, 0, len(tc.sd.Spec.Locations))

	for i := range tc.sd.Spec.Locations {
		serviceLocation := comp_v1.ServiceDescriptorLocation{
			Region:  tc.sd.Spec.Locations[i].Region,
			EnvType: tc.sd.Spec.Locations[i].EnvType,
			Account: tc.sd.Spec.Locations[i].Account,
			Name:    tc.sd.Spec.Locations[i].Name,
			Label:   tc.sd.Spec.Locations[i].Label,
		}

		nsName := generateNamespaceName(tc.sd.Name, serviceLocation.Label)
		objectInfos = append(objectInfos, FormationObjectInfo{
			Name:        nsName,
			Namespace:   nsName,
			ServiceName: voyager.ServiceName(tc.sd.Name),
			Location:    serviceLocation.VoyagerLocation(),
			Resources:   tc.transformedResources,
		})
	}

	fakeSd.On("CreateFormationObjectDef", mock.Anything).Return(objectInfos, nil)

	serviceDescriptorTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "service_descriptor_transitions_total",
			Help:      "Records the number of times a ServiceDescriptor transitions into a new condition",
		},
		[]string{"name", "type", "reason"},
	)

	testClock := tc.clock
	if testClock == nil {
		testClock = clock.NewFakeClock(time.Unix(0, 0))
	}

	cntrlr := &Controller{
		logger: logger,
		clock:  testClock,

		formationClient:   formationClient,
		compositionClient: compositionClient,
		sdTransformer:     &fakeSd,
		location: options.Location{
			Account: objectInfos[0].Location.Account,
			Region:  objectInfos[0].Location.Region,
			EnvType: objectInfos[0].Location.EnvType,
		},

		nsUpdater: namespaceUpdater,
		ldUpdater: ldUpdater,
		ldIndexer: ldInformer.GetIndexer(),
		nsIndexer: nsInformer.GetIndexer(),

		serviceDescriptorTransitionsCounter: serviceDescriptorTransitionsCounter,
	}

	// we don't control the workQueue, so we call Process directly
	pctx := &ctrl.ProcessContext{
		Logger: logger,
		Object: tc.sd,
	}

	tc.test(t, cntrlr, pctx, tc)
}

type fakeSdTransformer struct {
	mock.Mock
}

func (m *fakeSdTransformer) CreateFormationObjectDef(serviceDescriptor *comp_v1.ServiceDescriptor) ([]FormationObjectInfo, error) {
	args := m.Called(serviceDescriptor)
	return args.Get(0).([]FormationObjectInfo), args.Error(1)
}

func TestGenerateNamespaceName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		label voyager.Label
		want  string
	}{
		{
			name:  "foo",
			label: "",
			want:  "foo",
		},
		{
			name:  "foo",
			label: "bar",
			want:  "foo--bar",
		},
	}
	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := generateNamespaceName(c.name, c.label)
			assert.Equal(t, c.want, got)
		})
	}
}

func findCreatedLocationDescriptor(actions []kube_testing.Action) (*form_v1.LocationDescriptor, bool) {
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if ld, ok := action.GetObject().(*form_v1.LocationDescriptor); ok {
			return ld, true
		}
	}
	return nil, false
}

func findCreatedNamespace(actions []kube_testing.Action) (*core_v1.Namespace, bool) {
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if ns, ok := action.GetObject().(*core_v1.Namespace); ok {
			return ns, true
		}
	}
	return nil, false
}

func findCreatedRoleBindings(actions []kube_testing.Action) map[string]*rbac_v1.RoleBinding {
	result := make(map[string]*rbac_v1.RoleBinding)
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if rb, ok := action.GetObject().(*rbac_v1.RoleBinding); ok {
			result[rb.Name] = rb
		}
	}
	return result
}

func findUpdatedLocationDescriptor(actions []kube_testing.Action) (*form_v1.LocationDescriptor, bool) {
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if ld, ok := action.GetObject().(*form_v1.LocationDescriptor); ok {
			return ld, true
		}
	}
	return nil, false
}

func findUpdatedRoleBindings(actions []kube_testing.Action) map[string]*rbac_v1.RoleBinding {
	result := make(map[string]*rbac_v1.RoleBinding)
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if rb, ok := action.GetObject().(*rbac_v1.RoleBinding); ok {
			result[rb.Name] = rb
		}
	}
	return result
}

func findUpdatedServiceDescriptor(actions []kube_testing.Action) (*comp_v1.ServiceDescriptor, bool) {
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if sd, ok := action.GetObject().(*comp_v1.ServiceDescriptor); ok {
			return sd, true
		}
	}
	return nil, false
}
