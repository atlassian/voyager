package replication

import (
	"context"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	compclient_fake "github.com/atlassian/voyager/pkg/composition/client/fake"
	compInf "github.com/atlassian/voyager/pkg/composition/informer"
	k8s_testing "github.com/atlassian/voyager/pkg/k8s/testing"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestCreateServiceDescriptor(t *testing.T) {
	t.Parallel()

	tc := testCase{
		test: func(t *testing.T, cntrlr *Controller, pctx *ctrl.ProcessContext, tc *testCase) {
			desiredSD := &comp_v1.ServiceDescriptor{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       comp_v1.ServiceDescriptorResourceKind,
					APIVersion: comp_v1.ServiceDescriptorResourceVersion,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:            "test-sd",
					ResourceVersion: "123",
				},
				Spec: comp_v1.ServiceDescriptorSpec{
					Version: "1",
				},
			}
			pctx.Object = desiredSD
			_, err := cntrlr.Process(pctx)
			require.NoError(t, err)

			actions := tc.sdFake.Actions()

			sd, _ := findCreatedSD(actions)
			require.NotNil(t, sd)
		},
	}

	tc.run(t)
}

func TestUpdateServiceDescriptor(t *testing.T) {
	t.Parallel()

	existingSD := &comp_v1.ServiceDescriptor{
		// we explicitly exclude the typemeta to mimic the behavior
		// we see with typed objects having their Type meta removed
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			UID:  "the-sd-uid",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Version: "1",
		},
	}

	tc := testCase{
		sdObjects: []runtime.Object{existingSD},
		test: func(t *testing.T, cntrlr *Controller, pctx *ctrl.ProcessContext, tc *testCase) {
			desiredSD := &comp_v1.ServiceDescriptor{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       comp_v1.ServiceDescriptorResourceKind,
					APIVersion: comp_v1.ServiceDescriptorResourceVersion,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "test-sd",
					UID:  "the-sd-uid",
				},
				Spec: comp_v1.ServiceDescriptorSpec{
					Version: "2",
				},
			}

			pctx.Object = desiredSD
			_, err := cntrlr.Process(pctx)
			require.NoError(t, err)

			actions := tc.sdFake.Actions()

			sd, _ := findUpdatedSD(actions)
			require.NotNil(t, sd)
		},
	}

	tc.run(t)
}

func TestUpdateServiceDescriptorNoOp(t *testing.T) {
	t.Parallel()

	existingSD := &comp_v1.ServiceDescriptor{
		// we explicitly exclude the typemeta to mimic the behavior
		// we see with typed objects having their Type meta removed
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			UID:  "the-sd-uid",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Version: "1",
		},
	}

	tc := testCase{
		sdObjects: []runtime.Object{existingSD},
		test: func(t *testing.T, cntrlr *Controller, pctx *ctrl.ProcessContext, tc *testCase) {
			// just copy the existing one, but we do need to add the GVK
			// since incoming objects will have that set.
			newSD := existingSD.DeepCopy()
			newSD.SetGroupVersionKind(comp_v1.ServiceDescriptorGVK)
			pctx.Object = newSD

			_, err := cntrlr.Process(pctx)
			require.NoError(t, err)

			actions := tc.sdFake.Actions()

			_, exists := findUpdatedSD(actions)
			require.False(t, exists)
		},
	}

	tc.run(t)
}

func TestSkipReplicationOfExisting(t *testing.T) {
	t.Parallel()

	existingSD := &comp_v1.ServiceDescriptor{
		// we explicitly exclude the typemeta to mimic the behavior
		// we see with typed objects having their Type meta removed
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			UID:  "the-sd-uid",
			Annotations: map[string]string{
				ReplicateKey: "false",
			},
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Version: "1",
		},
	}

	tc := testCase{
		sdObjects: []runtime.Object{existingSD},
		test: func(t *testing.T, cntrlr *Controller, pctx *ctrl.ProcessContext, tc *testCase) {
			desiredSD := &comp_v1.ServiceDescriptor{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       comp_v1.ServiceDescriptorResourceKind,
					APIVersion: comp_v1.ServiceDescriptorResourceVersion,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "test-sd",
					UID:  "the-sd-uid",
				},
				Spec: comp_v1.ServiceDescriptorSpec{
					Version: "2",
				},
			}
			pctx.Object = desiredSD

			_, err := cntrlr.Process(pctx)
			require.NoError(t, err)

			actions := tc.sdFake.Actions()

			_, createdExists := findCreatedSD(actions)
			require.False(t, createdExists)
			_, updatedExists := findUpdatedSD(actions)
			require.False(t, updatedExists)
		},
	}

	tc.run(t)
}

func TestSkipReplicationOfDesired(t *testing.T) {
	t.Parallel()

	existingSD := &comp_v1.ServiceDescriptor{
		// we explicitly exclude the typemeta to mimic the behavior
		// we see with typed objects having their Type meta removed
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "test-sd",
			UID:  "the-sd-uid",
		},
		Spec: comp_v1.ServiceDescriptorSpec{
			Version: "1",
		},
	}

	tc := testCase{
		sdObjects: []runtime.Object{existingSD},
		test: func(t *testing.T, cntrlr *Controller, pctx *ctrl.ProcessContext, tc *testCase) {
			desiredSD := &comp_v1.ServiceDescriptor{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       comp_v1.ServiceDescriptorResourceKind,
					APIVersion: comp_v1.ServiceDescriptorResourceVersion,
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "test-sd",
					UID:  "the-sd-uid",
					Annotations: map[string]string{
						ReplicateKey: "false",
					},
				},
				Spec: comp_v1.ServiceDescriptorSpec{
					Version: "2",
				},
			}
			pctx.Object = desiredSD

			_, err := cntrlr.Process(pctx)
			require.NoError(t, err)

			actions := tc.sdFake.Actions()

			_, createdExists := findCreatedSD(actions)
			require.False(t, createdExists)
			_, updatedExists := findUpdatedSD(actions)
			require.False(t, updatedExists)
		},
	}

	tc.run(t)
}

type testCase struct {
	logger *zap.Logger

	sdObjects []runtime.Object
	sdFake    *kube_testing.Fake

	test func(*testing.T, *Controller, *ctrl.ProcessContext, *testCase)
}

func (tc *testCase) run(t *testing.T) {
	tc.logger = zaptest.NewLogger(t)

	compositionClient := compclient_fake.NewSimpleClientset(tc.sdObjects...)
	tc.sdFake = &compositionClient.Fake
	sdInformer := compInf.ServiceDescriptorInformer(compositionClient, 0)

	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	stage.StartWithChannel(sdInformer.Run)
	require.True(t, cache.WaitForCacheSync(ctx.Done(), sdInformer.HasSynced))

	cntrlr := &Controller{
		logger:         tc.logger,
		localInformer:  sdInformer,
		remoteInformer: sdInformer,
		localClient:    compositionClient,
	}

	pctx := &ctrl.ProcessContext{
		Logger: tc.logger,
	}

	tc.test(t, cntrlr, pctx, tc)
}

func TestValidateHashNoHash(t *testing.T) {
	t.Parallel()
	sd := comp_v1.ServiceDescriptor{}
	err := validateHash(&sd)
	require.NoError(t, err)
}

func TestValidateHashHashMatch(t *testing.T) {
	t.Parallel()
	sd := comp_v1.ServiceDescriptor{}
	err := validateHash(&sd)
	require.NoError(t, err)
}

func TestValidateHashNoMatch(t *testing.T) {
	t.Parallel()
	sd := comp_v1.ServiceDescriptor{
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				hashKey: "123",
			},
		},
	}
	err := validateHash(&sd)
	require.Error(t, err)
}

func TestStripResourceVersion(t *testing.T) {
	t.Parallel()
	sd := comp_v1.ServiceDescriptor{
		ObjectMeta: meta_v1.ObjectMeta{
			ResourceVersion: "123",
		},
	}
	stripped := stripResourceVersion(&sd)
	_, exists := stripped.ObjectMeta.Annotations["resourceVersion"]
	require.False(t, exists)
}

func findCreatedSD(actions []kube_testing.Action) (*comp_v1.ServiceDescriptor, bool) {
	for _, action := range k8s_testing.FilterCreateActions(actions) {
		if r, ok := action.GetObject().(*comp_v1.ServiceDescriptor); ok {
			return r, true
		}
	}
	return nil, false
}

func findUpdatedSD(actions []kube_testing.Action) (*comp_v1.ServiceDescriptor, bool) {
	for _, action := range k8s_testing.FilterUpdateActions(actions) {
		if r, ok := action.GetObject().(*comp_v1.ServiceDescriptor); ok {
			return r, true
		}
	}
	return nil, false
}
