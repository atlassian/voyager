package replication

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/sets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func getValidSdSpec() comp_v1.ServiceDescriptorSpec {
	return comp_v1.ServiceDescriptorSpec{
		Locations: []comp_v1.ServiceDescriptorLocation{
			{Name: "foo", Account: "a", Region: "b", EnvType: "staging"},
		},
		Config: []comp_v1.ServiceDescriptorConfigSet{
			// must allow arbitrary vars here (but not in other places)
			{Scope: "staging", Vars: map[string]interface{}{"foo": "bar"}},
		},
		ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
			{
				Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
				Locations: []comp_v1.ServiceDescriptorLocationName{"foo"},
				Resources: []comp_v1.ServiceDescriptorResource{
					{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
				},
			},
		},
	}
}

func objectToRawExtension(t *testing.T, obj interface{}) runtime.RawExtension {
	bytes, err := json.Marshal(obj)
	require.NoError(t, err)

	return runtime.RawExtension{
		Raw: bytes,
	}
}

func admitWithContextAndLogger(t *testing.T, admissionRequest *admissionv1beta1.AdmissionRequest) (resp *admissionv1beta1.AdmissionResponse, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	validator, err := setupValidator()
	require.NoError(t, err)

	currentLocation := voyager.ClusterLocation{Account: "a", Region: "b", EnvType: "staging"}
	ac := AdmissionContext{
		validator:       validator,
		CurrentLocation: currentLocation,
		ReplicatedLocations: sets.NewClusterLocation(
			currentLocation,
			voyager.ClusterLocation{Account: "a", Region: "c", EnvType: "staging"},
		),
	}

	return ac.servicedescriptorMutationAdmitFunc(ctx, zaptest.NewLogger(t), admissionv1beta1.AdmissionReview{Request: admissionRequest})
}

func getPatches(t *testing.T, resp *admissionv1beta1.AdmissionResponse) []util.Patch {
	require.NotEmpty(t, resp.PatchType)
	require.Equal(t, *resp.PatchType, admissionv1beta1.PatchTypeJSONPatch)
	var patches []util.Patch
	require.NoError(t, json.Unmarshal(resp.Patch, &patches))
	return patches
}

func TestValidSD(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
}

func TestRejectDeletion(t *testing.T) {
	t.Parallel()
	_, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Delete,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{},
			},
		}),
	})
	require.Error(t, err)
}

func TestRejectBadGVK(t *testing.T) {
	t.Parallel()
	badgvk := meta_v1.GroupVersionResource{
		Group:    "",
		Version:  "",
		Resource: "",
	}
	_, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  badgvk,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: getValidSdSpec(),
		}),
	})
	require.Error(t, err)
}

func TestInvalidLocationNameInResourceGroups(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "foo", Account: "a", Region: "b", EnvType: "staging"},
				},
				Config: []comp_v1.ServiceDescriptorConfigSet{
					// must allow arbitrary vars here (but not in other places)
					{Scope: "staging", Vars: map[string]interface{}{"foo": "bar"}},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("explode"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"foo", "here"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "location \"here\" not known for resourceGroup \"explode\""), resp)
}

func TestNoValidLocation(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "somewhere", Account: "a", Region: "b", EnvType: "dev"},
					{Name: "foo", Account: "a", Region: "b", EnvType: "staging"},
				},
				Config: []comp_v1.ServiceDescriptorConfigSet{
					// must allow arbitrary vars here (but not in other places)
					{Scope: "dev", Vars: map[string]interface{}{"foo": "bar"}},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"somewhere"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", `no resource groups defined for "b.staging (account: a)", no resource groups to be created for environment type "staging"`), resp)
}

func TestNoResourcesForEnvType(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "foo", Account: "a", Region: "b", EnvType: "dev"},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"foo"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})

	require.NoError(t, err)
	require.Equal(t, rejected("", `no resource groups to be created for environment type "staging"`), resp)
}

func TestFailIfLabels(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "foo", Account: "a", Region: "b", EnvType: "staging", Label: "bar"},
				},
				Config: []comp_v1.ServiceDescriptorConfigSet{
					// must allow arbitrary vars here (but not in other places)
					{Scope: "staging", Vars: map[string]interface{}{"foo": "bar"}},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"foo"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})

	require.NoError(t, err)
	require.Equal(t, rejected("", `labels are currently not supported (location: "foo")`), resp)
}

func TestInvalidLocation(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "foo", Account: "a", Region: "z", EnvType: "staging"},
				},
				Config: []comp_v1.ServiceDescriptorConfigSet{
					// must allow arbitrary vars here (but not in other places)
					{Scope: "staging", Vars: map[string]interface{}{"foo": "bar"}},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"foo"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})

	require.NoError(t, err)
	require.Equal(t, rejected("", `location "foo" (region: "z", account: "a") does not exist in "staging" environment, see go/micros2-locations`), resp)
}

func TestSDTooLarge(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object:    runtime.RawExtension{Raw: make([]byte, maxServiceDescriptorSizeBytes+1)},
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "ServiceDescriptor size 102401 exceeds the limit 102400"), resp)
}

func TestInvalidSDName(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "blah--derp",
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "ServiceDescriptor name should not contain --"), resp)
}

func TestInvalidSD(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object:    runtime.RawExtension{Raw: []byte(`{"spec":{"locations":1}}`)},
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "validation failure list:\nspec.locations in body must be of type array: \"number\""), resp)
}

func TestRejectUpdateForDeletedSD(t *testing.T) {
	t.Parallel()

	deletionTime := meta_v1.Now()
	modifiedSpec := getValidSdSpec()
	modifiedSpec.ResourceGroups[0].Name = "modified"

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				DeletionTimestamp: &deletionTime,
			},
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: modifiedSpec,
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "ServiceDescriptor spec cannot be updated during deletion"), resp)
}

func TestAdditionalProperties(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object:    runtime.RawExtension{Raw: []byte(`{"spec":{"locations":[{"rgion": "x"}], "resourcGroups":[]}}`)},
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", `validation failure list:
spec.resourcGroups in body is a forbidden property
spec.locations.rgion in body is a forbidden property
spec.locations.name in body is required
spec.locations.region in body is required
spec.locations.envType in body is required`), resp)
}

func TestMissingTimestamp(t *testing.T) {
	t.Parallel()
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "Please remove the mutationTimestamp from the submitted Service Descriptor"), resp)
}

func TestParseBadNewTimestamp(t *testing.T) {
	t.Parallel()

	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "NOT A TIMESTAMP",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", `mutationTimestamp annotation can't be parsed: parsing time "NOT A TIMESTAMP" as "2006-01-02T15:04:05Z07:00": cannot parse "NOT A TIMESTAMP" as "2006"`), resp)
}

func TestParseBadOldTimestamp(t *testing.T) {
	t.Parallel()

	_, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "NOT A TIMESTAMP",
				},
			},
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:01Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.Error(t, err)
}

func TestBackwardsTimestamp(t *testing.T) {
	t.Parallel()
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:01Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, rejected("", "You are attempting to override a Service Descriptor from 2009-11-10T22:00:00Z with an older version from 2009-11-10T22:00:01Z"), resp)
}

func TestEqualTimestamp(t *testing.T) {
	t.Parallel()
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
}

func TestAddAnnotationsWithExisting(t *testing.T) {
	t.Parallel()
	startTime, err := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
	require.NoError(t, err)
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					"something": "else",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
	patches := getPatches(t, resp)
	require.NotEmpty(t, patches)
	require.Len(t, patches, 2)

	assert.Equal(t, util.Add, patches[0].Operation)
	assert.Equal(t, "/metadata/annotations/"+updatedKey, patches[0].Path)
	assert.NotEmpty(t, patches[0].Value)
	assertSaneTime(t, startTime, patches[0].Value.(string))
	assert.Equal(t, util.Patch{
		Operation: util.Add,
		Path:      "/metadata/annotations/" + hashKey,
		Value:     "68976c26f994fba671f4e29c8a34b0da53f0dac5",
	}, patches[1])
}

func TestAddAnnotationsCreate(t *testing.T) {
	t.Parallel()
	startTime, err := time.Parse(time.RFC3339, time.Now().Format(time.RFC3339))
	require.NoError(t, err)
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{
					{Name: "foo", Account: "a", Region: "b", EnvType: "staging"},
				},
				Config: []comp_v1.ServiceDescriptorConfigSet{
					// must allow arbitrary vars here (but not in other places)
					{Scope: "staging", Vars: map[string]interface{}{"foo": "bar"}},
				},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      comp_v1.ServiceDescriptorResourceGroupName("group1"),
						Locations: []comp_v1.ServiceDescriptorLocationName{"foo"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{Name: "foo2", Type: "bar", Spec: &runtime.RawExtension{Raw: []byte(`{"a": "b"}`)}},
						},
					},
				},
			},
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
	patches := getPatches(t, resp)
	require.NotEmpty(t, patches)
	require.Len(t, patches, 1)
	patch := patches[0]

	assert.Equal(t, util.Add, patch.Operation)
	assert.Equal(t, "/metadata/annotations", patch.Path)
	value := patch.Value.(map[string]interface{})
	assert.NotEmpty(t, value[updatedKey])
	assertSaneTime(t, startTime, value[updatedKey].(string))
	// confirm our hash is different to above
	assert.Equal(t, "3a619f41b19fe334cc7330a143138b4a8b5e0bf3", value[hashKey])
}

func TestAddAnnotationsUpdateKeepsSubmittedTS(t *testing.T) {
	t.Parallel()
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:01Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: getValidSdSpec(),
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
	patches := getPatches(t, resp)
	require.NotEmpty(t, patches)
	require.Len(t, patches, 2)

	assert.Equal(t, util.Add, patches[0].Operation)
	assert.Equal(t, "/metadata/annotations/"+updatedKey, patches[0].Path)
	assert.NotEmpty(t, patches[0].Value)
	assert.Equal(t, "2009-11-10T22:00:01Z", patches[0].Value.(string))
	assert.Equal(t, util.Patch{
		Operation: util.Add,
		Path:      "/metadata/annotations/" + hashKey,
		Value:     "68976c26f994fba671f4e29c8a34b0da53f0dac5",
	}, patches[1])
}

func TestEditExistingSD(t *testing.T) {
	t.Parallel()
	resp, err := admitWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:01Z",
				},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Version: "3",
				Config: []comp_v1.ServiceDescriptorConfigSet{{
					Scope: "global",
					Vars: map[string]interface{}{
						"test-var-group": map[string]string{
							"test-var-1": "test-var-1-value",
						},
					},
				}},
				Locations: []comp_v1.ServiceDescriptorLocation{{
					Name:    "test-location",
					EnvType: "staging",
					Account: "a",
					Region:  "b",
				}},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      "test-group",
						Locations: []comp_v1.ServiceDescriptorLocationName{"test-location"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{
								Name: "test-resource-1",
								Type: "test-resource-type",
								Spec: &runtime.RawExtension{Raw: []byte(`{"test": "${test-var-group.test-var-1}"}`)},
							},
						},
					},
				},
			},
		}),
		OldObject: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Annotations: map[string]string{
					updatedKey: "2009-11-10T22:00:00Z",
				},
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Version: "3",
				Config: []comp_v1.ServiceDescriptorConfigSet{{
					Scope: "global",
					Vars: map[string]interface{}{
						"test-var-group": map[string]string{
							"test-var-1": "test-var-1-value",
							"test-var-2": "test-var-2-value",
						},
					},
				}},
				Locations: []comp_v1.ServiceDescriptorLocation{{
					Name:    "test-location",
					EnvType: "staging",
					Account: "a",
					Region:  "b",
				}},
				ResourceGroups: []comp_v1.ServiceDescriptorResourceGroup{
					{
						Name:      "test-group",
						Locations: []comp_v1.ServiceDescriptorLocationName{"test-location"},
						Resources: []comp_v1.ServiceDescriptorResource{
							{
								Name: "test-resource-1",
								Type: "test-resource-type",
								Spec: &runtime.RawExtension{Raw: []byte(`{"test": "${test-var-group.test-var-1}"}`)},
							},
							{
								Name: "test-resource-2",
								Type: "test-resource-type",
								Spec: &runtime.RawExtension{Raw: []byte(`{"test": "${test-var-group.test-var-2}"}`)},
							},
						},
					},
				},
			},
		}),
	})
	require.NoError(t, err)
	assert.True(t, resp.Allowed)
}

func assertSaneTime(t *testing.T, startTime time.Time, timeString string) {
	patchTime, err := parseTimestamp(timeString)
	require.NoError(t, err)
	endTime := time.Now()
	assert.True(t, patchTime.Equal(startTime) || patchTime.After(startTime), patchTime.Format(time.RFC3339))
	assert.True(t, patchTime.Equal(endTime) || patchTime.Before(endTime), patchTime.Format(time.RFC3339))
}
