package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/admission"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var stateResource = metav1.GroupVersionResource{
	Group:    orch_v1.SchemeGroupVersion.Group,
	Version:  orch_v1.SchemeGroupVersion.Version,
	Resource: orch_v1.StateResourcePlural,
}

func SetupAdmissionWebhooks(r *chi.Mux) {
	r.Post(
		"/admission/statevalidation",
		admission.AdmitFuncHandlerFunc("statevalidation", stateValidationAdmitFunc))
}

func stateValidationAdmitFunc(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	admissionRequest := admissionReview.Request

	// Validate supported operations
	if admissionRequest.Operation != admissionv1beta1.Create && admissionRequest.Operation != admissionv1beta1.Update {
		return nil, errors.Errorf("only create or update is supported, got %q", admissionRequest.Operation)
	}

	if admissionRequest.Resource != stateResource {
		return nil, errors.Errorf("only State is supported, got %q", admissionRequest.Resource)
	}

	state := orch_v1.State{}
	if err := json.Unmarshal(admissionRequest.Object.Raw, &state); err != nil {
		return nil, errors.WithStack(err)
	}

	return admitState(&state)
}

func admitState(state *orch_v1.State) (*admissionv1beta1.AdmissionResponse, error) {
	// Very dumb TODO - this validation code should probably exist somewhere else
	// to avoid the admission webhook having the entire validation impl here.

	// For a better errorlist implementation, maybe take a look at
	// kubernetes apimachinery/pkg/util/validation/field/errors.go
	var errorlist []string
	resourceNames := make(map[voyager.ResourceName]struct{})
	for _, stateResource := range state.Spec.Resources {
		if _, ok := resourceNames[stateResource.Name]; ok {
			errorlist = append(errorlist, fmt.Sprintf("duplicate resource name %q", stateResource.Name))
		} else {
			resourceNames[stateResource.Name] = struct{}{}
		}
	}

	if len(errorlist) > 0 {
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: strings.Join(errorlist, ", "),
				Code:    http.StatusUnprocessableEntity,
				Reason:  metav1.StatusReasonInvalid,
			},
		}, nil
	}
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: "no validation errors found in State",
		},
	}, nil
}
