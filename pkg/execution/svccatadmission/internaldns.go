package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atlassian/voyager/pkg/microsserver"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns"
	"github.com/atlassian/voyager/pkg/servicecentral"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InternalDNSAdmitFunc checks existing DNS alias ownership via micros server API, for both InternalDNS Services and Ingress Resources
func InternalDNSAdmitFunc(ctx context.Context, microsServerClient *microsserver.Client, scClient serviceCentralClient, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {

	admissionRequest := admissionReview.Request

	// Validate supported resource type
	if admissionRequest.Resource != serviceInstanceResource || admissionRequest.Resource != ingressResourceType {
		return nil, errors.Errorf("unsupported resource, got %v", admissionRequest.Resource)
	}

	serviceName := getServiceNameFromNamespace(admissionRequest.Namespace)
	namespaceService, err := getServiceData(ctx, scClient, serviceName)
	if err != nil {
		return nil, errors.Errorf("error fetching service central data for namespace %v", admissionRequest.Namespace)
	}

	switch admissionRequest.Resource {
	case ingressResourceType:
		ingress := &v1beta1.Ingress{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, ingress); err != nil {
			return nil, errors.Errorf("error decoding ingress resource")
		}
		for _, ingressRule := range ingress.Spec.Rules {
			response, err := isDomainAllowedToMigrade(ctx, microsServerClient, scClient, namespaceService, ingressRule.Host)
			if err != nil {
				return nil, err
			}
			if response.Allowed == false {
				return response, nil
			}
		}
	case serviceInstanceResource:
		serviceInstance := sc_v1b1.ServiceInstance{}
		if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
			return nil, errors.Wrap(err, "malformed ServiceInstance specification")
		}
		var internalDNSSpec internaldns.Spec
		if err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &internalDNSSpec); err != nil {
			return nil, errors.Wrap(err, "error parsing InternalDNS specs")
		}
		for _, alias := range internalDNSSpec.Aliases {
			response, err := isDomainAllowedToMigrade(ctx, microsServerClient, scClient, namespaceService, alias.Name)
			if err != nil {
				return nil, err
			}
			if response.Allowed == false {
				return response, nil
			}
		}
	}

	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: "allowed domainName",
		},
	}, nil

}

func isDomainAllowedToMigrade(ctx context.Context, microsServerClient *microsserver.Client, scClient serviceCentralClient, namespaceService *servicecentral.ServiceData, domainName string) (*admissionv1beta1.AdmissionResponse, error) {
	aliasInfo, err := microsServerClient.GetAlias(ctx, domainName)
	if err != nil {
		return &admissionv1beta1.AdmissionResponse{}, errors.Wrap(err, "error requesting alias info from micros server")
	}
	if aliasInfo != (&microsserver.AliasInfo{}) { // response not empty, existing dns alias
		if namespaceService.ServiceOwner.Username != aliasInfo.Service.Owner {
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf(
						"requested dns alias %q is currently owned by %q via service %q, and can not be migrated to service %q owned by different owner %q",
						domainName,
						aliasInfo.Service.Owner,
						aliasInfo.Service.Name,
						namespaceService.ServiceName,
						namespaceService.ServiceOwner.Username,
					),
					Code: http.StatusForbidden,
				},
			}, nil
		}
	}
	return &admissionv1beta1.AdmissionResponse{}, nil
}
