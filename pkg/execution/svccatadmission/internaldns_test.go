package svccatadmission

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns/api"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
)

func TestInternalDNSAdmitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	microsServerMock := setupMicrosServerMock()
	serviceCentralMock := setupSCMock()

	tests := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		want            *admissionv1beta1.AdmissionResponse
		wantErr         bool
	}{
		{
			"internaldns new.domain",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"internaldns multiple new.domain",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new1.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new2.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new3.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new4.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new5.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new6.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"internaldns registered domain same user",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "doug.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"internaldns multiple with one registered domain same user",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "doug.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new2.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new3.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new4.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new5.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new6.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"internaldns registered domain different user",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "elsie.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and can not be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
		{
			"internaldns multiple with one registered domain different user",
			buildAdmissionReview(dougComputeService, serviceInstanceResource, admissionv1beta1.Create, buildServiceInstance(
				t, apiinternaldns.ClusterServiceClassExternalID, apiinternaldns.ClusterServicePlanExternalID, apiinternaldns.Spec{
					Aliases: []apiinternaldns.Alias{
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "elsie.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new2.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new3.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new4.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new5.domain",
						},
						{
							AliasType: apiinternaldns.AliasTypeSimple,
							Name:      "new6.domain",
						},
					},
				}),
			),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and can not be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
		{
			"ingress new.domain",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{"new.domain"})),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress multiple new.domain",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{
				"new1.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress registered domain same user",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{"doug.domain"})),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress multiple with one registered domain same user",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{
				"doug.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(true, 0, nil, `requested domain name(s) allowed for use`),
			false,
		},
		{
			"ingress registered domain different user",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{"elsie.domain"})),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and can not be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
		{
			"ingress multiple with one registered domain different user",
			buildAdmissionReview(dougComputeService, ingressResource, admissionv1beta1.Create, buildIngress(t, []string{
				"elsie.domain",
				"new2.domain",
				"new3.domain",
				"new4.domain",
				"new5.domain",
				"new6.domain",
			})),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `requested dns alias "elsie.domain" is currently owned by "elsie" via service "elsie-compute-service", and can not be migrated to service "doug-compute-service" owned by different owner "doug"`),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InternalDNSAdmitFunc(ctx, microsServerMock, serviceCentralMock, tc.admissionReview)
			if (err != nil) != tc.wantErr {
				t.Errorf("InternalDNSAdmitFunc() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("InternalDNSAdmitFunc() = %v, want %v", got, tc.want)
			}
		})
	}

}
