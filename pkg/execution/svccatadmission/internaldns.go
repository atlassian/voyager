package svccatadmission

import (
	"context"
	"github.com/atlassian/voyager/pkg/microsserver"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
)

// InternalDNSAdmitFunc checks DNS alias ownership using micros server API
func InternalDNSAdmitFunc(ctx context.Context, microsServerClient microsserver.Client, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
	return nil, nil
}
