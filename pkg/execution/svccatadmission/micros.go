package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atlassian/voyager"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	microsClusterServiceClassExternalID   = "1e524b0d-2877-47a2-b41e-ddcd881d85da"
	microsClusterServiceClassExternalName = "micros"
	microsClusterServiceClassName         = microsClusterServiceClassExternalID

	microsV1ClusterServicePlanExternalID   = "272d49a6-57ef-466d-963f-0a84c5258aec"
	microsV1ClusterServicePlanExternalName = "default-plan"
	microsV1ClusterServicePlanName         = microsV1ClusterServicePlanExternalID
)

func MicrosAdmitFunc(ctx context.Context, scClient serviceCentralClient, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	if admissionRequest.Resource != serviceInstanceResource {
		return nil, errors.Errorf("unsupported resource, got %v", admissionRequest.Resource)
	}

	serviceInstance := sc_v1b1.ServiceInstance{}
	if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
		return nil, errors.WithStack(err)
	}

	if !IsMicrosServiceClass(serviceInstance) {
		reason := fmt.Sprintf(
			"ServiceInstance %q is not a micros compute instance",
			serviceInstance.Name)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Message: reason,
			},
		}, nil
	}

	switch admissionRequest.Operation {
	case admissionv1beta1.Create:
		return validateMicrosCreate(ctx, scClient, admissionRequest.Namespace, serviceInstance)
	case admissionv1beta1.Update:
		oldServiceInstance := sc_v1b1.ServiceInstance{}

		if err := json.Unmarshal(admissionRequest.OldObject.Raw, &oldServiceInstance); err != nil {
			return nil, errors.WithStack(err)
		}

		return validateMicrosUpdate(oldServiceInstance, serviceInstance)
	default:
		return nil, errors.Errorf("only create and update are supported, got %q", admissionRequest.Operation)
	}
}

func IsMicrosServiceClass(serviceInstance sc_v1b1.ServiceInstance) bool {
	return serviceInstance.Spec.ClusterServiceClassName == microsClusterServiceClassName ||
		serviceInstance.Spec.ClusterServiceClassExternalID == microsClusterServiceClassExternalID ||
		serviceInstance.Spec.ClusterServiceClassExternalName == microsClusterServiceClassExternalName
}

func isV1Plan(serviceInstance sc_v1b1.ServiceInstance) bool {
	return serviceInstance.Spec.ClusterServicePlanName == microsV1ClusterServicePlanName ||
		serviceInstance.Spec.ClusterServicePlanExternalID == microsV1ClusterServicePlanExternalID ||
		serviceInstance.Spec.ClusterServicePlanExternalName == microsV1ClusterServicePlanExternalName
}

func getServiceName(serviceInstance sc_v1b1.ServiceInstance) (voyager.ServiceName, error) {
	if isV1Plan(serviceInstance) {
		var v1PlanParameters struct {
			Name voyager.ServiceName `json:"name"`
		}
		if err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &v1PlanParameters); err != nil {
			return "", errors.WithStack(err)
		}
		return v1PlanParameters.Name, nil
	}

	// This is V2 for now, but we'll hope for forwards compatibility...
	var otherPlanParameters struct {
		Service struct {
			ID voyager.ServiceName `json:"id"`
		} `json:"service"`
	}
	if err := json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &otherPlanParameters); err != nil {
		return "", errors.WithStack(err)
	}

	return otherPlanParameters.Service.ID, nil
}

// validateMicrosCreate makes it harder to steal other people's services.
//
// To do this, we compare the 'compute' service owner (i.e. micros service name)
// with the voyager service owner (the owner of the namespace).
//
// For future (better?) plans, see: https://trello.com/c/6C4N8LC8/
//
// Note that we give 'Results' with a Message even on Allowed true here, because
// it helps our logging situation, even though Kubernetes currently ignores
// this unless Allowed is false.
func validateMicrosCreate(ctx context.Context, scClient serviceCentralClient, namespace string, serviceInstance sc_v1b1.ServiceInstance) (*admissionv1beta1.AdmissionResponse, error) {
	name, err := getServiceName(serviceInstance)
	if err != nil {
		return nil, err
	}

	if name == "" {
		reason := "no service id/name specified for micros ServiceInstance"
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusBadRequest,
			},
		}, nil
	}

	response, err := allowedToMigrate(ctx, scClient, namespace, name)
	if err != nil {
		return nil, err
	}

	if response == nil {
		// If the service doesn't yet exist, it's fine - we'll create it (and own it).
		// Race here that's unlikely to matter...
		reason := fmt.Sprintf(
			"compute service %q doesn't exist in Service Central", name)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Message: reason,
			},
		}, nil
	}

	return response, nil
}

// validateMicrosUpdate checks to see if the service name changes
//
// This is sufficient to prevent people stealing resources provided our Create check
// has worked (and avoids extra calls to Service Central).
func validateMicrosUpdate(oldServiceInstance, newServiceInstance sc_v1b1.ServiceInstance) (*admissionv1beta1.AdmissionResponse, error) {
	oldName, err := getServiceName(oldServiceInstance)
	if err != nil {
		return nil, err
	}
	newName, err := getServiceName(newServiceInstance)
	if err != nil {
		return nil, err
	}

	if newName != oldName {
		reason := fmt.Sprintf("not allowed to change micros compute name (%q -> %q)", oldName, newName)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusBadRequest,
			},
		}, nil
	}

	reason := fmt.Sprintf("service name didn't change (%q)", oldName)
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: reason,
		},
	}, nil
}
