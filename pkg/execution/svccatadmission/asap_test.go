package svccatadmission

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/atlassian/voyager/pkg/k8s"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func genASAPKeyRawSpec(t *testing.T, serviceName string) []byte {
	rawSpecMap := sc_v1b1.ServiceInstance{}
	rawSpecMap.Spec.ClusterServiceClassName = ASAPKeyClusterServiceClassName
	rawSpecMap.Spec.Parameters = &runtime.RawExtension{
		Raw: []byte("{\"serviceName\":\"" + serviceName + "\"}"),
	}
	rawSpec, err := json.Marshal(rawSpecMap)
	require.NoError(t, err)
	return rawSpec
}

func TestAsapKeyAdmitFunc(t *testing.T) {

	t.Parallel()

	tests := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		want            *admissionv1beta1.AdmissionResponse
	}{
		{
			"serviceName matching namespace",
			buildAdmissionReview("foo", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "foo")),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, "serviceName is prefixed by namespace"),
		},
		{
			"serviceName prefixed by namespace",
			buildAdmissionReview("foo", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "foo/bar")),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, "serviceName is prefixed by namespace"),
		},
		{
			"namespace with label",
			buildAdmissionReview("foo--dev", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "foo")),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, "serviceName is prefixed by namespace"),
		},
		{
			"namespace with label and serviceName with ASAPKey resource name",
			buildAdmissionReview("foo--dev", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "foo/bar")),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, "serviceName is prefixed by namespace"),
		},
		{
			"namespace with label and serviceName with ASAPKey resource name mismatch",
			buildAdmissionReview("foo--dev", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "bar/foo")),
			buildAdmissionResponse(false, http.StatusForbidden, metav1.StatusReasonForbidden, nil, `serviceName was set to "bar/foo", which is not prefixed by namespace "foo--dev"`),
		},
		{
			"serviceName not prefixed by namespace",
			buildAdmissionReview("foo", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "bar/foo")),
			buildAdmissionResponse(false, http.StatusForbidden, metav1.StatusReasonForbidden, nil, `serviceName was set to "bar/foo", which is not prefixed by namespace "foo"`),
		},
	}

	testsError := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		wantErr         string
	}{
		{
			"with no namespace",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Create, genASAPKeyRawSpec(t, "whatever")),
			"no namespace in AdmissionReview request",
		},
	}

	ctx := context.Background()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AsapKeyAdmitFunc(ctx, tc.admissionReview)
			require.NoError(t, err)
			require.Equal(t, tc.want.Result.Message, got.Result.Message)
			require.Equal(t, tc.want.Allowed, got.Allowed)
		})
	}

	for _, tc := range testsError {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AsapKeyAdmitFunc(ctx, tc.admissionReview)
			require.EqualError(t, err, tc.wantErr)
		})
	}

}
