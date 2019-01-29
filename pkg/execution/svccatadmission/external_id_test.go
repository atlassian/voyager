package svccatadmission

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	rps_testing "github.com/atlassian/voyager/pkg/execution/svccatadmission/rps/testing"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/util/uuid"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ uuid.Generator = &uuidGeneratorStub{}
)

type uuidGeneratorStub struct {
	Value string
}

func (g *uuidGeneratorStub) NewUUID() string {
	return g.Value
}

func TestExternalUUIDAdmitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scStore := setupSCMock()

	const fakeUUID = "541db78e-8d5f-4b22-b70f-f892afc53e05"
	uuidStub := &uuidGeneratorStub{
		Value: fakeUUID,
	}

	tests := []struct {
		name            string
		admissionReview admissionv1beta1.AdmissionReview
		want            *admissionv1beta1.AdmissionResponse
		wantErr         bool
	}{
		{
			"no external id for ServiceInstance",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Create, []byte(`{"spec":{"externalId": ""}}`)),
			buildAdmissionResponse(true, 0, []byte(`\[{"op":"add","path":"/spec/externalID","value":"[^"]+"}\]`), ""),
			false,
		},
		{
			"no external id for ServiceBinding",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Create, []byte(`{"spec":{"externalId": ""}}`)),
			buildAdmissionResponse(true, 0, []byte(`\[{"op":"add","path":"/spec/externalID","value":"[^"]+"}\]`), ""),
			false,
		},
		{
			"external id for ServiceInstance",
			buildAdmissionReview("", k8s.ServiceInstanceGVR, admissionv1beta1.Create, []byte(`{"spec":{"externalId": "foo"}}`)),
			buildAdmissionResponse(true, 0, nil, ""),
			false,
		},
		{
			"external id for ServiceInstance in RPS with wrong user",
			buildAdmissionReview(dougMicros2Service, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstanceWithExternalID(t, "9a3f2d35-0ce8-48b7-8531-a72b5cd02fd4")),
			buildAdmissionResponse(false, http.StatusUnauthorized, nil, `service central owner of service "elsie-compute-service" (elsie) is different to micros2 service "doug-micros2-service" (doug)`),
			false,
		},
		{
			"external id for ServiceInstance in RPS with missing namespace service",
			buildAdmissionReview(missingService, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstanceWithExternalID(t, "9a3f2d35-0ce8-48b7-8531-a72b5cd02fd4")),
			buildAdmissionResponse(false, http.StatusUnauthorized, nil, `namespace service "missing-service" does not exist in Service Central - should be impossible`),
			false,
		},
		{
			"external id for ServiceInstance in RPS with no resource service",
			buildAdmissionReview(dougMicros2Service, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstanceWithExternalID(t, "d0aea45f-7718-4bfe-9c89-1aaf5a668161")),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `for instanceId "d0aea45f-7718-4bfe-9c89-1aaf5a668161" RPS claims owning service is "missing-service", but that doesn't exist in service central`),
			false,
		},
		{
			"external id for ServiceInstance in RPS with right user",
			buildAdmissionReview(elsieMicros2Service, k8s.ServiceInstanceGVR, admissionv1beta1.Create, buildServiceInstanceWithExternalID(t, "9a3f2d35-0ce8-48b7-8531-a72b5cd02fd4")),
			buildAdmissionResponse(true, 0, nil, `good migration wooh`),
			false,
		},
		{
			"external id for ServiceBinding",
			buildAdmissionReview(elsieMicros2Service, k8s.ServiceBindingGVR, admissionv1beta1.Create, []byte(`{"spec":{"externalId": "foo"}}`)),
			buildAdmissionResponse(false, http.StatusForbidden, nil, `externalID was set by user to "foo", expected empty`),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rpsCache := rps_testing.MockRPSCache(t)

			got, err := ExternalUUIDAdmitFunc(ctx, uuidStub, scStore, rpsCache, tc.admissionReview)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ExternalUUIDAdmitFunc() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got.Allowed != true {
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("ExternalUUIDAdmitFunc() = %v, want %v", got, tc.want)
				}
			} else {
				// if it's Allowed, we're attempting a patch with a UUID, which
				// is trickier to test...
				assert.Equal(t, tc.want.Allowed, got.Allowed)
				assert.Equal(t, tc.want.PatchType, got.PatchType)
				assert.Regexp(t, string(tc.want.Patch), string(got.Patch))
			}
		})
	}
}

func buildServiceInstanceWithExternalID(t *testing.T, externalID string) []byte {
	rawServiceInstance, err := json.Marshal(sc_v1b1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceInstanceName,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ExternalID: externalID,
		},
	})
	require.NoError(t, err)
	return rawServiceInstance
}
