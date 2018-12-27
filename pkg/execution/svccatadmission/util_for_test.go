package svccatadmission

import (
	"context"
	"fmt"

	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/mock"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	missingService      = "missing-service"
	dougMicros2Service  = "doug-micros2-service"
	dougComputeService  = "doug-compute-service"
	elsieMicros2Service = "elsie-micros2-service"
	elsieComputeService = "elsie-compute-service"
)

type serviceCentralClientMock struct {
	mock.Mock
}

var _ serviceCentralClient = &serviceCentralClientMock{}

func (m *serviceCentralClientMock) ListServices(ctx context.Context, user auth.OptionalUser, serviceName string) ([]servicecentral.ServiceData, error) {
	args := m.Called(ctx, user, serviceName)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return []servicecentral.ServiceData{result.(servicecentral.ServiceData)}, args.Error(1)
}

func setupSCMock() serviceCentralClient {
	makeSearchString := func(serviceName string) string {
		return fmt.Sprintf("service_name='%s'", serviceName)
	}
	scStore := new(serviceCentralClientMock)
	scStore.
		On("ListServices", mock.Anything, auth.NoUser(), makeSearchString(missingService)).
		Return(nil, nil)
	scStore.
		On("ListServices", mock.Anything, auth.NoUser(), makeSearchString(dougComputeService)).
		Return(servicecentral.ServiceData{
			ServiceName: dougComputeService,
			ServiceOwner: servicecentral.ServiceOwner{
				Username: "doug",
			},
		}, nil)
	scStore.
		On("ListServices", mock.Anything, auth.NoUser(), makeSearchString(dougMicros2Service)).
		Return(servicecentral.ServiceData{
			ServiceName: dougMicros2Service,
			ServiceOwner: servicecentral.ServiceOwner{
				Username: "doug",
			},
		}, nil)
	scStore.
		On("ListServices", mock.Anything, auth.NoUser(), makeSearchString(elsieComputeService)).
		Return(servicecentral.ServiceData{
			ServiceName: elsieComputeService,
			ServiceOwner: servicecentral.ServiceOwner{
				Username: "elsie",
			},
		}, nil)
	scStore.
		On("ListServices", mock.Anything, auth.NoUser(), makeSearchString(elsieMicros2Service)).
		Return(servicecentral.ServiceData{
			ServiceName: elsieMicros2Service,
			ServiceOwner: servicecentral.ServiceOwner{
				Username: "elsie",
			},
		}, nil)

	return scStore
}

func buildAdmissionReview(namespace string, resource string, operation admissionv1beta1.Operation, rawSpec []byte) admissionv1beta1.AdmissionReview {
	return admissionv1beta1.AdmissionReview{
		Request: &admissionv1beta1.AdmissionRequest{
			Namespace: namespace,
			Operation: operation,
			Resource: metav1.GroupVersionResource{
				Group:    sc_v1b1.SchemeGroupVersion.Group,
				Version:  sc_v1b1.SchemeGroupVersion.Version,
				Resource: resource,
			},
			Object: runtime.RawExtension{
				Raw: rawSpec,
			},
		},
	}
}

func buildAdmissionResponse(allowed bool, code int32, patch []byte, message string) *admissionv1beta1.AdmissionResponse {
	pt := admissionv1beta1.PatchTypeJSONPatch
	admissionResponse := &admissionv1beta1.AdmissionResponse{
		Allowed: allowed,
		Result: &metav1.Status{
			Message: message,
			Code:    code,
		},
	}
	if patch != nil {
		admissionResponse.Patch = patch
		admissionResponse.PatchType = &pt
	}
	return admissionResponse
}
