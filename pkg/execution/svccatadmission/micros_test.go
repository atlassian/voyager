package svccatadmission

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	serviceInstanceName = "foo"
	defaultNamespace    = "somenamespace"
)

func TestMicrosAdmitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scStore := setupSCMock()

	tests := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		want            *admissionv1beta1.AdmissionResponse
		wantErr         bool
	}{
		{
			"NotServiceInstance",
			buildAdmissionReview(defaultNamespace, serviceBinding, admissionv1beta1.Create, []byte(`{}`)),
			nil,
			true,
		},
		{
			"ServiceInstanceNotMicros",
			buildAdmissionReview(dougMicros2Service, serviceInstance, admissionv1beta1.Create, buildServiceInstance(t, "serviceid", "planid", nil)),
			buildAdmissionResponse(true, 0, nil, `ServiceInstance "foo" is not a micros compute instance`),
			false,
		},
		{
			"ErrorIfNoNamespace",
			buildAdmissionReview("", serviceInstance, admissionv1beta1.Create, buildV1ServiceInstance(t, missingService)),
			nil,
			true,
		},
		{
			"ServiceMissingIsOk",
			buildAdmissionReview(dougMicros2Service, serviceInstance, admissionv1beta1.Create, buildV1ServiceInstance(t, missingService)),
			buildAdmissionResponse(true, 0, nil, "compute service \"missing-service\" doesn't exist in Service Central"),
			false,
		},
		{
			"ServiceHasSameOwner",
			buildAdmissionReview(elsieMicros2Service, serviceInstance, admissionv1beta1.Create, buildV1ServiceInstance(t, elsieComputeService)),
			buildAdmissionResponse(true, 0, nil, `service central owner of service "elsie-compute-service" (elsie) is same as micros2 service "elsie-micros2-service" (elsie)`),
			false,
		},
		{
			"ServiceHasDifferentOwnerForbidden",
			buildAdmissionReview(elsieMicros2Service, serviceInstance, admissionv1beta1.Create, buildV1ServiceInstance(t, dougComputeService)),
			buildAdmissionResponse(false, http.StatusUnauthorized, nil, `service central owner of service "doug-compute-service" (doug) is different to micros2 service "elsie-micros2-service" (elsie)`),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MicrosAdmitFunc(ctx, scStore, tc.admissionReview)
			if (err != nil) != tc.wantErr {
				t.Errorf("MicrosAdmitFunc() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MicrosAdmitFunc() = %v, want %v", got, tc.want)
			}
		})
	}
}

func buildV1ServiceInstance(t *testing.T, serviceName string) []byte {
	type Parameters struct {
		Name string `json:"name"`
	}
	return buildServiceInstance(
		t, microsClusterServiceClassName, microsV1ClusterServicePlanName,
		Parameters{serviceName})
}

func buildOtherServiceInstance(t *testing.T, serviceName string) []byte {
	type Service struct {
		ID string `json:"id"`
	}

	type Parameters struct {
		Service Service `json:"service"`
	}

	return buildServiceInstance(
		t, microsClusterServiceClassName, "foo",
		Parameters{Service{serviceName}})
}

func buildServiceInstance(t *testing.T, serviceClass, servicePlan string, parameters interface{}) []byte {
	rawParameters, err := json.Marshal(parameters)
	require.NoError(t, err)
	rawServiceInstance, err := json.Marshal(sc_v1b1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
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
