package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util/auth"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	missingService      = "missing-service"
	dougMicros2Service  = "doug-micros2-service"
	dougComputeService  = "doug-compute-service"
	elsieMicros2Service = "elsie-micros2-service"
	elsieComputeService = "elsie-compute-service"
	dougComputeDomain   = "doug.domain"
	elsieComputeDomain  = "elsie.domain"
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

type microsServerClientMock struct {
	mock.Mock
}

func (m *microsServerClientMock) GetAlias(ctx context.Context, domainName string) (*microsserver.AliasInfo, error) {
	args := m.Called(ctx, domainName)
	result := args.Get(0)
	if result == nil {
		return nil, nil
	}
	return args.Get(0).(*microsserver.AliasInfo), nil
}

func setupMicrosServerMock() *microsServerClientMock {
	microsServerMock := new(microsServerClientMock)
	dougRegisteredAliasInfo := &microsserver.AliasInfo{
		Service: microsserver.Service{
			Name:  dougComputeService,
			Owner: "doug",
		},
	}
	elsieRegisteredAliasInfo := &microsserver.AliasInfo{
		Service: microsserver.Service{
			Name:  elsieComputeService,
			Owner: "elsie",
		},
	}
	microsServerMock.On("GetAlias", mock.Anything, dougComputeDomain).Return(dougRegisteredAliasInfo)
	microsServerMock.On("GetAlias", mock.Anything, elsieComputeDomain).Return(elsieRegisteredAliasInfo)
	microsServerMock.On("GetAlias", mock.Anything, mock.Anything).Return(nil)
	return microsServerMock
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

func buildIngress(t *testing.T, hosts []string) []byte {
	var rules []ext_v1b1.IngressRule
	for _, host := range hosts {
		rules = append(rules, ext_v1b1.IngressRule{
			Host: host,
			IngressRuleValue: ext_v1b1.IngressRuleValue{
				HTTP: &ext_v1b1.HTTPIngressRuleValue{
					Paths: []ext_v1b1.HTTPIngressPath{
						{
							Path: "/",
							Backend: ext_v1b1.IngressBackend{
								ServiceName: "anyService",
								ServicePort: intstr.FromInt(8080),
							},
						},
					},
				},
			},
		},
		)
	}
	rawIngress, err := json.Marshal(&ext_v1b1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       k8s.IngressKind,
			APIVersion: ext_v1b1.SchemeGroupVersion.String(),
		},
		Spec: ext_v1b1.IngressSpec{
			Rules: rules,
		},
	})
	require.NoError(t, err)
	return rawIngress
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
