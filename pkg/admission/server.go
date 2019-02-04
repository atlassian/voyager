package admission

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	ctrllogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/sets"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var (
	kubesystemAccounts = sets.NewString(
		"system:serviceaccount:kube-system:generic-garbage-collector",
		"system:serviceaccount:kube-system:namespace-controller",
	)
)

func AdmitFuncHandlerFunc(admitFuncName string, admitFunc AdmitFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := logz.RetrieveLoggerFromContext(ctx)

		var body []byte
		if r.Body != nil {
			if data, err := ioutil.ReadAll(r.Body); err == nil {
				body = data
			}
		}

		// verify the content type is accurate
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			logger.Sugar().Errorf("unexpected contentType=%s", contentType)
			handleErrorf(logger, w, "contentType=%s, expect application/json", contentType)
			return
		}

		// unmarshal the AdmissionReview from the body
		admissionReview := v1beta1.AdmissionReview{}
		if err := json.Unmarshal(body, &admissionReview); err != nil {
			logger.Error("AdmissionReview unmarshal error", zap.Error(err))
			handleError(logger, w, "could not unmarshal AdmissionReview from body", err)
			return
		}
		if admissionReview.Request == nil {
			logger.Error("admissionReview.Request is nil")
			handleErrorf(logger, w, "request admissionReview did not have request object")
			return
		}

		// blanket exempt garbage collector and namespace-controller from our webhooks
		if kubesystemAccounts.Has(admissionReview.Request.UserInfo.Username) {
			response := &v1beta1.AdmissionResponse{
				Allowed: true,
				Result: &metav1.Status{
					Message: "kube-system accounts allowed through webhook",
				},
			}
			writeResponse(logger, w, response, admissionReview.Request.UID)
			return
		}

		gvk := admissionReview.Request.Kind

		reviewContext{
			Context: ctx,
			Logger: logger.With(
				ctrllogz.NamespaceName(admissionReview.Request.Namespace),
				ctrllogz.ObjectName(admissionReview.Request.Name),
				ctrllogz.ObjectGk(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}),
			),
			AdmissionReview: admissionReview,
		}.CallHandlerFunc(w, admitFuncName, admitFunc)
	}
}

func handleErrorf(logger *zap.Logger, w http.ResponseWriter, template string, args ...interface{}) {
	errString := fmt.Sprintf(template, args...)
	response := toErrorAdmissionResponse(errString)
	writeResponse(logger, w, response, types.UID(""))
}

func handleError(logger *zap.Logger, w http.ResponseWriter, msg string, err error) {
	response := toErrorAdmissionResponse(err.Error())
	writeResponse(logger, w, response, types.UID(""))
}

func toErrorAdmissionResponse(errString string) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: errString,
			Code:    http.StatusInternalServerError,
		},
	}
}

func writeResponse(logger *zap.Logger, w http.ResponseWriter, response *v1beta1.AdmissionResponse, uid types.UID) {
	review := v1beta1.AdmissionReview{}
	if response != nil {
		review.Response = response
		review.Response.UID = uid
	}

	resp, err := json.Marshal(review)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to marshall response %v", review), zap.Error(errors.WithStack(err)))
		httputil.WriteIntenalServerErrorResponse(logger, w, err)
	}

	httputil.WriteOkResponse(logger, w, resp)
}
