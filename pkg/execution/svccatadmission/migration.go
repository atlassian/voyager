package svccatadmission

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func allowedToMigrate(ctx context.Context, scClient serviceCentralClient, namespace string, resourceServiceName string) (*admissionv1beta1.AdmissionResponse, error) {

	namespaceServiceName := getServiceNameFromNamespace(namespace)

	if namespaceServiceName == "" {
		// This happened in the past because the namespace wasn't always in the ServiceInstance
		// object. Now we're getting it from the admission request, and it should always
		// be there. But, just in case...
		// (amusingly, Service Central thinks sbennett is the owner of "" - i.e.
		// sbennett had all the power!)
		return nil, errors.New("unable to find service name in namespace")
	}

	if resourceServiceName == namespaceServiceName {
		// If the names are identical, then of course you have permission
		// (no need to check Service Central).
		reason := fmt.Sprintf(
			"Resource service owner %q has same name as micros2 service %q",
			resourceServiceName, namespaceServiceName)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Message: reason,
			},
		}, nil
	}

	resourceService, err := getServiceData(ctx, scClient, resourceServiceName)
	if err != nil {
		return nil, err
	}

	if resourceService == nil {
		// Returning nil means we don't have sufficient information to fulfil the request
		// (i.e. the service does not exist). The caller can decide what to do about this.
		return nil, nil
	}

	namespaceService, err := getServiceData(ctx, scClient, namespaceServiceName)
	if err != nil {
		return nil, err
	}

	if namespaceService == nil {
		reason := fmt.Sprintf(
			"namespace service %q does not exist in Service Central - should be impossible",
			namespaceServiceName)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusUnauthorized,
			},
		}, nil
	}

	if resourceService.ServiceOwner.Username != namespaceService.ServiceOwner.Username {
		reason := fmt.Sprintf(
			"service central owner of service %q (%s) is different to micros2 service %q (%s)",
			resourceServiceName, resourceService.ServiceOwner.Username,
			namespaceServiceName, namespaceService.ServiceOwner.Username)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusUnauthorized,
			},
		}, nil
	}

	reason := fmt.Sprintf(
		"service central owner of service %q (%s) is same as micros2 service %q (%s)",
		resourceServiceName, resourceService.ServiceOwner.Username,
		namespaceServiceName, namespaceService.ServiceOwner.Username)
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: reason,
		},
	}, nil
}
