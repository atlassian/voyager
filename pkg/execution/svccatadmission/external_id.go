package svccatadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/uuid"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ExternalUUIDAdmitFunc(ctx context.Context, uuidGenerator uuid.Generator, scClient serviceCentralClient, rpsCache *rps.Cache, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	// Validate supported operations
	if admissionRequest.Operation != admissionv1beta1.Create {
		return nil, errors.Errorf("only create is supported, got %q", admissionRequest.Operation)
	}

	// Try to handle the resource
	switch admissionRequest.Resource {
	case k8s.ServiceInstanceGVR:
		return handleExternalIDServiceInstance(ctx, uuidGenerator, scClient, rpsCache, admissionRequest)
	case k8s.ServiceBindingGVR:
		return handleExternalIDServiceBinding(uuidGenerator, admissionRequest)
	default:
		return nil, errors.Errorf("unsupported resource, got %v", admissionRequest.Resource)
	}
}

func handleExternalIDServiceInstance(ctx context.Context, uuidGenerator uuid.Generator, scClient serviceCentralClient, rpsCache *rps.Cache, admissionRequest *admissionv1beta1.AdmissionRequest) (*admissionv1beta1.AdmissionResponse, error) {
	serviceInstance := sc_v1b1.ServiceInstance{}

	if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceInstance); err != nil {
		return nil, errors.WithStack(err)
	}

	externalID := serviceInstance.Spec.ExternalID
	if externalID != "" {
		return allowedToMigrateFromRPS(ctx, scClient, rpsCache, admissionRequest.Namespace, externalID)
	}

	return patchExternalID(uuidGenerator)
}

func handleExternalIDServiceBinding(uuidGenerator uuid.Generator, admissionRequest *admissionv1beta1.AdmissionRequest) (*admissionv1beta1.AdmissionResponse, error) {
	serviceBinding := sc_v1b1.ServiceBinding{}

	if err := json.Unmarshal(admissionRequest.Object.Raw, &serviceBinding); err != nil {
		return nil, errors.WithStack(err)
	}

	externalID := serviceBinding.Spec.ExternalID
	if externalID != "" {
		reason := fmt.Sprintf("externalID was set by user to %q, expected empty", externalID)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusForbidden,
			},
		}, nil
	}

	return patchExternalID(uuidGenerator)
}

func allowedToMigrateFromRPS(ctx context.Context, scClient serviceCentralClient, rpsCache *rps.Cache, namespace string, externalID string) (*admissionv1beta1.AdmissionResponse, error) {
	resourceServiceName, err := rpsCache.GetServiceFor(ctx, externalID)
	if err != nil {
		return nil, err
	}

	if resourceServiceName == "" {
		reason := fmt.Sprintf("externalID was set by user to %q but not in RPS for migration. Temporarily allowing to enable soft undelete (VYGR-258)", externalID)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true, // TODO: undo VYGR-258
			Result: &metav1.Status{
				Message: reason,
			},
		}, nil
	}

	response, err := allowedToMigrate(ctx, scClient, namespace, resourceServiceName)
	if err != nil {
		return nil, err
	}

	if response == nil {
		reason := fmt.Sprintf("for instanceId %q RPS claims owning service is %q, but that doesn't exist in service central",
			externalID, resourceServiceName)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: reason,
				Code:    http.StatusForbidden,
			},
		}, nil
	}

	return response, nil
}

func patchExternalID(uuidGenerator uuid.Generator) (*admissionv1beta1.AdmissionResponse, error) {
	// Service Catalog will actually handle setting the UUID for us, but let's
	// make it a legit mutating controller and claim that everything is handled
	// here (this also means we work more nicely if there's another mutating
	// admission controller which wants to mess with externalID - otherwise
	// it might 'skip' our validation).
	// For more information, see the MutatingWebhookConfiguration in k8s-deployments
	externalID := uuidGenerator.NewUUID()
	patch, err := json.Marshal(util.JSONPatch{
		util.Patch{
			Operation: util.Add,
			Path:      "/spec/externalID",
			Value:     externalID,
		},
	})
	if err != nil {
		return nil, err
	}

	pt := admissionv1beta1.PatchTypeJSONPatch
	reason := fmt.Sprintf("Patched ExternalID with %q", externalID)
	return &admissionv1beta1.AdmissionResponse{
		Allowed:   true,
		Patch:     patch,
		PatchType: &pt,
		Result: &metav1.Status{
			Message: reason,
		},
	}, nil
}
