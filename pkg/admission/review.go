package admission

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"k8s.io/api/admission/v1beta1"
)

type AdmitFunc func(context.Context, *zap.Logger, v1beta1.AdmissionReview) (*v1beta1.AdmissionResponse, error)

type reviewContext struct {
	Context         context.Context
	Logger          *zap.Logger
	AdmissionReview v1beta1.AdmissionReview
}

func (r reviewContext) CallHandlerFunc(w http.ResponseWriter, name string, f AdmitFunc) {
	// run the admitFunc
	r.Logger.Sugar().Debugf("calling admitFunc with review %v", r.AdmissionReview)
	admissionResponse, err := f(r.Context, r.Logger, r.AdmissionReview)
	if err != nil {
		r.writeErrorResponse(w, "could not run admitFunc", err)
		return
	}
	if admissionResponse == nil {
		r.writeErrorfResponse(w, "%q returned empty admissionResponse", name)
		return
	}
	admissionResponse.AuditAnnotations = generateWebhookAnnotations(admissionResponse)

	r.writeResponse(w, admissionResponse)
}

func (r reviewContext) writeErrorResponse(w http.ResponseWriter, msg string, err error) {
	r.Logger.Error(msg, zap.Error(err))
	response := toErrorAdmissionResponse(strings.Join([]string{msg, err.Error()}, ":"))
	r.writeResponse(w, response)
}

func (r reviewContext) writeErrorfResponse(w http.ResponseWriter, template string, args ...interface{}) {
	errString := fmt.Sprintf(template, args...)
	r.Logger.Error(errString)
	response := toErrorAdmissionResponse(errString)
	r.writeResponse(w, response)
}

func (r reviewContext) writeResponse(w http.ResponseWriter, response *v1beta1.AdmissionResponse) {
	if response.Allowed {
		r.Logger.Sugar().Debugf("responding with allowed: %v and result %v", response.Allowed, response.Result)
	} else {
		r.Logger.Sugar().Infof("responding with allowed: %v and result %v", response.Allowed, response.Result)
	}
	writeResponse(r.Logger, w, response, r.AdmissionReview.Request.UID)
}
