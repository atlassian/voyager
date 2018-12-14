package admission

import (
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
)

const (
	decisionAnnotationKey = "decision"
	reasonAnnotationKey   = "reason"
	decisionAllow         = "allow"
	decisionForbid        = "forbid"
)

func generateWebhookAnnotations(response *admissionv1beta1.AdmissionResponse) map[string]string {
	return map[string]string{
		decisionAnnotationKey: decisionValue(response),
		reasonAnnotationKey:   reasonValue(response),
	}
}

func decisionValue(response *admissionv1beta1.AdmissionResponse) string {
	if response.Allowed {
		return decisionAllow
	}
	return decisionForbid
}

func reasonValue(response *admissionv1beta1.AdmissionResponse) string {
	if response.Result == nil {
		return ""
	}
	return response.Result.Message
}
