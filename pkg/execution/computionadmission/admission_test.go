package computionadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/k8s"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	admission_v1beta1 "k8s.io/api/admission/v1beta1"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	apis_metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	k8s_fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

const (
	serviceInstanceName = "foo"
	defaultNamespace    = "somenamespace"
)

type admissionResponseCase struct {
	input   []string
	want    *admission_v1beta1.AdmissionResponse
	wantErr error
}

func TestIsCompliantArtifact(t *testing.T) {
	t.Parallel()
	ac := AdmissionContext{
		CompliantDockerPrefixes: []string{
			`docker.example.com/sox/`,
			`docker-registry-v2.us-west-1.example.com/sox/`,
			`docker-registry-v2.us-east-1.example.com/sox/`,
		}}
	type artifactCase struct {
		input string
		want  bool
	}
	cases := []artifactCase{
		{
			input: "",
			want:  false,
		},
		{
			input: "nginx",
			want:  false,
		},
		{
			input: "hub.docker.com/nginx",
			want:  false,
		},
		{
			input: "subdomain.docker.example.com/sox/nginx",
			want:  false,
		},
		{
			input: "docker.example.com/sox/nginx",
			want:  true,
		},
		{
			input: "docker-registry-v2.us-east-1.example.com/sox/nginx",
			want:  true,
		},
	}
	for ti, tc := range cases {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			got := ac.isCompliantArtifact(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSuccessResponseForArtifacts(t *testing.T) {
	t.Parallel()
	ac := AdmissionContext{
		CompliantDockerPrefixes: []string{
			`docker.example.com/sox/`,
		}}
	cases := []admissionResponseCase{
		{
			input: []string{},
			want: &admission_v1beta1.AdmissionResponse{
				Allowed: true,
				Result: &apis_metav1.Status{
					Message: reasonPRGBCompliantArtifacts,
				},
			},
		},
		{
			input: []string{
				"docker.example.com/sox/nginx",
				"docker.example.com/sox/redis",
			},
			want: &admission_v1beta1.AdmissionResponse{
				Allowed: true,
				Result: &apis_metav1.Status{
					Message: reasonPRGBCompliantArtifacts,
				},
			},
		},
	}
	for ti, tc := range cases {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			got, err := ac.responseForArtifacts(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFailureResponseForArtifacts(t *testing.T) {
	t.Parallel()
	ac := AdmissionContext{
		CompliantDockerPrefixes: []string{
			`docker.example.com/sox/`,
		}}

	cases := []admissionResponseCase{
		{
			input: []string{
				"nginx",
				"hub.docker.com/nginx",
			},
			want: rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "nginx,hub.docker.com/nginx")),
		},
		{
			input: []string{
				"nginx",
				"docker.example.com/sox/nginx",
				"hub.docker.com/nginx",
			},
			want: rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "nginx,hub.docker.com/nginx")),
		},
		{
			input: []string{
				"docker.example.com/sox/nginx",
				"nginx",
				"hub.docker.com/nginx",
			},
			want: rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "nginx,hub.docker.com/nginx")),
		},
	}
	for ti, tc := range cases {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			got, err := ac.responseForArtifacts(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBasicAdmitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name            string
		act             AdmissionContext
		admissionReview admission_v1beta1.AdmissionReview
		want            *admission_v1beta1.AdmissionResponse
		wantErr         error
	}{
		{
			"NonPRGBComplianceEnvironment",
			AdmissionContext{
				EnforcePRGB: false,
				CompliantDockerPrefixes: []string{
					`docker.example.com/sox/`,
				},
			},
			admission_v1beta1.AdmissionReview{
				Request: &admission_v1beta1.AdmissionRequest{
					Namespace: defaultNamespace,
					Operation: admission_v1beta1.Create,
					Resource: apis_metav1.GroupVersionResource{
						Group:    sc_v1b1.SchemeGroupVersion.Group,
						Version:  sc_v1b1.SchemeGroupVersion.Version,
						Resource: "serviceinstances",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{}`),
					},
				},
			},
			&admission_v1beta1.AdmissionResponse{
				Allowed: true,
				Result: &apis_metav1.Status{
					Message: reasonUnrestrictedEnvironment,
				},
			},
			nil,
		},
		{
			"EmptyNamespaceAdmissionRequest",
			AdmissionContext{
				EnforcePRGB: true,
				CompliantDockerPrefixes: []string{
					`docker.example.com/sox/`,
				},
			},
			admission_v1beta1.AdmissionReview{
				Request: &admission_v1beta1.AdmissionRequest{
					Namespace: "",
					Operation: admission_v1beta1.Create,
					Resource: apis_metav1.GroupVersionResource{
						Group:    sc_v1b1.SchemeGroupVersion.Group,
						Version:  sc_v1b1.SchemeGroupVersion.Version,
						Resource: "serviceinstances",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{}`),
					},
				},
			},
			&admission_v1beta1.AdmissionResponse{
				Allowed: true,
				Result: &apis_metav1.Status{
					Message: reasonNonNamespacedResource,
				},
			},
			nil,
		},
	}

	logger := zaptest.NewLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.act.serviceInstanceAdmitFunc(ctx, logger, tc.admissionReview)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("serviceInstanceAdmitFunc() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNonVoyagerNamespace(t *testing.T) {
	t.Parallel()
	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				"test-non-voyager-label": "test",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "data"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: []byte(`{}`),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: true,
			Result: &apis_metav1.Status{
				Message: reasonUnrestrictedNamespace,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestNamespaceNotExist(t *testing.T) {
	t.Parallel()
	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "random",
			Labels: map[string]string{
				"test-non-voyager-label": "test",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "data"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: []byte(`{}`),
				},
			},
		},
		want:       nil,
		wantErr:    errors.New("failed to validate Namespace: somenamespace: Namespace doesn't exist inside informer indexer"),
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestComplianceNotRequired(t *testing.T) {
	t.Parallel()
	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "testService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: false\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: buildServiceInstance(t, "1e524b0d-2877-47a2-b41e-ddcd881d85da", "v2", nil),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: true,
			Result: &apis_metav1.Status{
				Message: reasonPRGBComplianceNotRequired,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)
}

func TestConfigMapMissing(t *testing.T) {
	t.Parallel()
	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "testService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "random-cfm",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: false\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: buildServiceInstance(t, "1e524b0d-2877-47a2-b41e-ddcd881d85da", "v2", nil),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &apis_metav1.Status{
				Message: reasonServiceMetaConfigMapMissing,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)
}

func TestConfigMapFormatWrong(t *testing.T) {
	t.Parallel()
	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "testService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "wrong format of config data."},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: buildServiceInstance(t, "1e524b0d-2877-47a2-b41e-ddcd881d85da", "v2", nil),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: false,
			Result: &apis_metav1.Status{
				Message: reasonWrongFormatServiceMetaData,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)
}

func TestServiceInstanceAdminSuccess(t *testing.T) {
	t.Parallel()

	css := composeServices{
		Docker: dockerCompose{
			Compose: map[string]partialContainerSpec{
				"serviceA": partialContainerSpec{
					Image: "docker.example.com/sox/nginx",
				},
			},
		},
	}

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: buildServiceInstance(t, "1e524b0d-2877-47a2-b41e-ddcd881d85da", "v2", css),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: true,
			Result: &apis_metav1.Status{
				Message: reasonPRGBCompliantArtifacts,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)
}

func TestServiceInstanceAdminFailure(t *testing.T) {
	t.Parallel()

	css := composeServices{
		Docker: dockerCompose{
			Compose: map[string]partialContainerSpec{
				"serviceA": partialContainerSpec{
					Image: "docker.example.com/sox/nginx",
				},
				"serviceB": partialContainerSpec{
					Image: "docker.example.com/not-sox/nginx",
				},
			},
		},
	}

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.ServiceInstanceKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "serviceinstances",
				},
				Object: runtime.RawExtension{
					Raw: buildServiceInstance(t, "1e524b0d-2877-47a2-b41e-ddcd881d85da", "v2", css),
				},
			},
		},
		want:       rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "docker.example.com/not-sox/nginx")),
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestPodAdminSuccess(t *testing.T) {
	t.Parallel()

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.PodKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Kind: meta_v1.GroupVersionKind{
					Group:   core_v1.GroupName,
					Version: "v1",
					Kind:    k8s.PodKind,
				},
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "pods",
				},
				Object: runtime.RawExtension{
					Raw: buildPod(t, "docker.example.com/sox/nginx", "docker.example.com/sox/abc"),
				},
			},
		},
		want: &admission_v1beta1.AdmissionResponse{
			Allowed: true,
			Result: &apis_metav1.Status{
				Message: reasonPRGBCompliantArtifacts,
			},
		},
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestPodAdminFailure(t *testing.T) {
	t.Parallel()

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.PodKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Kind: meta_v1.GroupVersionKind{
					Group:   core_v1.GroupName,
					Version: "v1",
					Kind:    k8s.PodKind,
				},
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    sc_v1b1.SchemeGroupVersion.Group,
					Version:  sc_v1b1.SchemeGroupVersion.Version,
					Resource: "pods",
				},
				Object: runtime.RawExtension{
					Raw: buildPod(t, "docker.example.com/sox/nginx", "docker.example.com/badimage/abc"),
				},
			},
		},
		want:       rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "docker.example.com/badimage/abc")),
		wantErr:    nil,
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestFailsOnUnknownGVK(t *testing.T) {
	t.Parallel()

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	tc := testCase{
		EnforcePRGB: true,
		target:      k8s.DeploymentKind,
		request: admission_v1beta1.AdmissionReview{
			Request: &admission_v1beta1.AdmissionRequest{
				Kind: meta_v1.GroupVersionKind{
					Group:   apps_v1.GroupName,
					Version: "vNext", // this isn't a real version
					Kind:    k8s.DeploymentKind,
				},
				Namespace: defaultNamespace,
				Operation: admission_v1beta1.Create,
				Resource: apis_metav1.GroupVersionResource{
					Group:    apps_v1.GroupName,
					Version:  "vNext",
					Resource: "deployments",
				},
				Object: runtime.RawExtension{
					Raw: buildPod(t, "docker.example.com/sox/nginx", "docker.example.com/badimage/abc"),
				},
			},
		},
		want:       nil,
		wantErr:    errors.New(`unknown GVK of "apps/vNext, Kind=Deployment" provided`),
		nsObjects:  []runtime.Object{nsObj},
		cfmObjects: []runtime.Object{cfmObj},
	}
	tc.run(t)

}

func TestDeploymentAdminSuccess(t *testing.T) {
	t.Parallel()

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}

	// want to cover multiple deployment types
	types := []schema.GroupVersionKind{
		k8s.DeploymentGVK,
		ext_v1beta1.SchemeGroupVersion.WithKind(k8s.DeploymentKind),
	}

	for _, deploymentType := range types {
		t.Run("Type: "+deploymentType.String(), func(t *testing.T) {
			tc := testCase{
				EnforcePRGB: true,
				target:      k8s.DeploymentKind,
				request: admission_v1beta1.AdmissionReview{
					Request: &admission_v1beta1.AdmissionRequest{
						Kind: meta_v1.GroupVersionKind{
							Group:   deploymentType.Group,
							Version: deploymentType.Version,
							Kind:    deploymentType.Kind,
						},
						Namespace: defaultNamespace,
						Operation: admission_v1beta1.Create,
						Resource: apis_metav1.GroupVersionResource{
							Group:    deploymentType.Group,
							Version:  deploymentType.Version,
							Resource: "Deployments",
						},
						Object: runtime.RawExtension{
							Raw: buildDeployment(t, "docker.example.com/sox/nginx", "docker.example.com/sox/abc"),
						},
					},
				},
				want: &admission_v1beta1.AdmissionResponse{
					Allowed: true,
					Result: &apis_metav1.Status{
						Message: reasonPRGBCompliantArtifacts,
					},
				},
				wantErr:    nil,
				nsObjects:  []runtime.Object{nsObj},
				cfmObjects: []runtime.Object{cfmObj},
			}
			tc.run(t)
		})
	}
}

func TestDeploymentAdminFailure(t *testing.T) {
	t.Parallel()

	nsObj := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				voyager.ServiceNameLabel: "voygerService",
			},
		},
		Spec:   core_v1.NamespaceSpec{},
		Status: core_v1.NamespaceStatus{},
	}
	cfmObj := &core_v1.ConfigMap{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service-metadata",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{"config": "businessUnit: Paas/Other\ncompliance:\n  prgbControl: true\nloggingId: 02a10b6\nnotifications:\n  email: abc@example.com\n  lowPriority:\n    cloudwatch: https://events.pagerduty.com/adapter/sns\n    generic: edeff9c76\n  main:\n    cloudwatch: https://events.xxx\n    generic: 422edeff9c76\nresourceOwner: testuser\nssamAccessLevel: paas-testuser-test-svc-dl-dev\n"},
	}
	// want to cover multiple deployment types
	types := []schema.GroupVersionKind{
		k8s.DeploymentGVK,
		ext_v1beta1.SchemeGroupVersion.WithKind(k8s.DeploymentKind),
	}

	for _, deploymentType := range types {
		t.Run("Type: "+deploymentType.String(), func(t *testing.T) {
			tc := testCase{
				EnforcePRGB: true,
				target:      k8s.DeploymentKind,
				request: admission_v1beta1.AdmissionReview{
					Request: &admission_v1beta1.AdmissionRequest{
						Kind: meta_v1.GroupVersionKind{
							Group:   deploymentType.Group,
							Version: deploymentType.Version,
							Kind:    deploymentType.Kind,
						},
						Namespace: defaultNamespace,
						Operation: admission_v1beta1.Create,
						Resource: apis_metav1.GroupVersionResource{
							Group:    deploymentType.Group,
							Version:  deploymentType.Version,
							Resource: "Deployments",
						},
						Object: runtime.RawExtension{
							Raw: buildDeployment(t, "docker.example.com/sox/nginx", "docker.example.com/badimage/abc"),
						},
					},
				},
				want:       rejectWithReason(fmt.Sprintf("%s: %q", reasonNonCompliantArtifactFound, "docker.example.com/badimage/abc")),
				wantErr:    nil,
				nsObjects:  []runtime.Object{nsObj},
				cfmObjects: []runtime.Object{cfmObj},
			}
			tc.run(t)
		})
	}
}

type testCase struct {
	EnforcePRGB bool
	target      string
	request     admission_v1beta1.AdmissionReview
	want        *admission_v1beta1.AdmissionResponse
	wantErr     error
	nsObjects   []runtime.Object
	cfmObjects  []runtime.Object
}

func (ctc *testCase) run(t *testing.T) {
	nsClient := k8s_fake.NewSimpleClientset(ctc.nsObjects...)
	cfmClient := k8s_fake.NewSimpleClientset(ctc.cfmObjects...)

	nsInformer := core_v1inf.NewNamespaceInformer(nsClient, 0, cache.Indexers{})
	cfmInformer := core_v1inf.NewConfigMapInformer(cfmClient, meta_v1.NamespaceAll, 0, cache.Indexers{})

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	// Start all informers then wait on them
	for _, inf := range []cache.SharedIndexInformer{nsInformer, cfmInformer} {
		stage.StartWithChannel(inf.Run)
	}
	for _, inf := range []cache.SharedIndexInformer{nsInformer, cfmInformer} {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	act := AdmissionContext{
		ConfigMapInformer: cfmInformer,
		NamespaceInformer: nsInformer,
		EnforcePRGB:       ctc.EnforcePRGB,
		CompliantDockerPrefixes: []string{
			`docker.example.com/sox/`,
		},
	}
	logger := zaptest.NewLogger(t)
	if ctc.target == k8s.ServiceInstanceKind {
		got, err := act.serviceInstanceAdmitFunc(ctx, logger, ctc.request)
		if ctc.wantErr == nil {
			require.NoError(t, err)
			assert.Equal(t, ctc.want, got)
		} else {
			assert.EqualError(t, ctc.wantErr, err.Error())
		}

	} else {
		got, err := act.podAdmitFunc(ctx, logger, ctc.request)
		if ctc.wantErr == nil {
			require.NoError(t, err)
			assert.Equal(t, ctc.want, got)
		} else {
			assert.EqualError(t, ctc.wantErr, err.Error())
		}
	}

}

func buildServiceInstance(t *testing.T, serviceClass, servicePlan string, parameters interface{}) []byte {
	rawParameters, err := json.Marshal(parameters)
	require.NoError(t, err)
	rawServiceInstance, err := json.Marshal(sc_v1b1.ServiceInstance{
		ObjectMeta: apis_metav1.ObjectMeta{
			Name: serviceInstanceName,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassName: serviceClass,
				ClusterServicePlanName:  servicePlan,
			},
			Parameters: &runtime.RawExtension{
				Raw: rawParameters,
			},
		},
	})
	require.NoError(t, err)
	return rawServiceInstance
}

func buildPodSpec(t *testing.T, images []string) core_v1.PodSpec {
	var containers []core_v1.Container
	for i, image := range images {
		containers = append(containers, core_v1.Container{
			Name:  fmt.Sprintf("container%d", i),
			Image: image,
		})
	}
	return core_v1.PodSpec{
		InitContainers: []core_v1.Container{},
		Containers:     containers,
	}
}

func buildPod(t *testing.T, images ...string) []byte {
	pod := core_v1.Pod{
		ObjectMeta: apis_metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: defaultNamespace,
		},
		Spec: buildPodSpec(t, images),
	}
	rawPod, err := json.Marshal(pod)
	require.NoError(t, err)
	return rawPod
}

func buildDeployment(t *testing.T, images ...string) []byte {
	d := apps_v1.Deployment{
		ObjectMeta: apis_metav1.ObjectMeta{
			Name:      "testdeployment",
			Namespace: defaultNamespace,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: buildPodSpec(t, images),
			},
		},
	}
	rawDeployment, err := json.Marshal(d)
	require.NoError(t, err)
	return rawDeployment
}
