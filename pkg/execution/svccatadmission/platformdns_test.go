package svccatadmission

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/atlassian/voyager/pkg/k8s"
	apiplatformdns "github.com/atlassian/voyager/pkg/orchestration/wiring/platformdns/api"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPlatformDNSAdmitFunc(t *testing.T) {
	t.Parallel()
	ctx := logz.CreateContextWithLogger(context.Background(), zap.NewNop())
	microsServerMock := setupMicrosServerMock()
	serviceCentralMock := setupSCMock()

	tests := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		want            *admissionv1beta1.AdmissionResponse
		wantErr         bool
	}{
		{
			"platformdns new.domain",
			buildAdmissionReview(dougComputeService, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstance(
				t, apiplatformdns.ClusterServiceClassExternalID, apiplatformdns.ClusterServicePlanExternalID, apiplatformdns.Spec{
					AliasType: apiplatformdns.AliasTypeSimple,
					Name:      "new.domain",
				}),
			),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"platformdns registered domain same user",
			buildAdmissionReview(dougComputeService, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstance(
				t, apiplatformdns.ClusterServiceClassExternalID, apiplatformdns.ClusterServicePlanExternalID, apiplatformdns.Spec{
					AliasType: apiplatformdns.AliasTypeSimple,
					Name:      "doug.domain",
				}),
			),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"platformdns registered domain different user",
			buildAdmissionReview(dougComputeService, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstance(
				t, apiplatformdns.ClusterServiceClassExternalID, apiplatformdns.ClusterServicePlanExternalID, apiplatformdns.Spec{
					AliasType: apiplatformdns.AliasTypeSimple,
					Name:      "elsie.domain",
				}),
			),
			buildAdmissionResponse(false, http.StatusForbidden, metav1.StatusReasonForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and cannot be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
		{
			"not platformdns create",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstance(
				t, "otherClassExternalID", "otherPlanExternalID", apiplatformdns.Spec{}),
			),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested ServiceInstance is not "PlatformDNS" type`),
			false,
		},
		{
			"not platformdns update",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Update, buildServiceInstance(
				t, "otherClassExternalID", "otherPlanExternalID", apiplatformdns.Spec{}),
			),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested ServiceInstance is not "PlatformDNS" type`),
			false,
		},
		{
			"ingress new.domain",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{"new.domain"})),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress multiple new.domain",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{
				"new1.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress registered domain same user",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{"doug.domain"})),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress multiple with one registered domain same user",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{
				"doug.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(true, 0, metav1.StatusReasonUnknown, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress registered domain different user",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{"elsie.domain"})),
			buildAdmissionResponse(false, http.StatusForbidden, metav1.StatusReasonForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and cannot be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
		{
			"ingress multiple with one registered domain different user",
			buildAdmissionReview(dougComputeService, k8s.IngressGVR, admissionv1beta1.Create, buildIngress(t, []string{
				"elsie.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(false, http.StatusForbidden, metav1.StatusReasonForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and cannot be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlatformDNSAdmitFunc(ctx, microsServerMock, serviceCentralMock, tc.admissionReview)
			if (err != nil) != tc.wantErr {
				require.Equal(t, tc.wantErr, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				require.Equal(t, tc.want, got)
			}
		})
	}

}
