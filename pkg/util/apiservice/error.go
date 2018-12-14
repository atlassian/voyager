package apiservice

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Using meta_v1.Status will allow kubectl to display errors
type ErrorStatus meta_v1.Status

func (e *ErrorStatus) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, int(e.Code))
	return nil
}

// newErrorStatusRenderer will create a Kubernetes APIMachinery Status object that is our reply to caller. Note: if
// someone is using `kubectl`, they'll see the string from err.Error().
func newErrorStatusRenderer(code int32, reason meta_v1.StatusReason, message string, reqID string) render.Renderer {
	return &ErrorStatus{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Status",
			APIVersion: "v1",
		},
		ListMeta: meta_v1.ListMeta{},
		Status:   meta_v1.StatusFailure,
		Reason:   reason,
		Code:     code,

		// Message is shown to the user.
		Message: fmt.Sprintf("%s: %s", reqID, message),
	}
}

func respond(logger *zap.Logger, w http.ResponseWriter, r *http.Request, code int32, reason meta_v1.StatusReason, message string, cause error) {
	reqID := middleware.GetReqID(r.Context())
	if code == http.StatusInternalServerError {
		logger.Error(message, zap.Error(cause))
	} else {
		logger.Info(message, zap.Error(cause))
	}
	renderer := newErrorStatusRenderer(code, reason, message, reqID)
	if err := render.Render(w, r, renderer); err != nil {
		logger.Error("error rendering response", zap.String("original_message", message), zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)

		// Single quoted instead of %q because it formats better for the user.
		fmt.Fprintf(w, "An error occurred while rendering an error '%s' to your request '%s'", message, reqID) // nolint: errcheck
	}
}

func RespondWithBadRequest(logger *zap.Logger, w http.ResponseWriter, r *http.Request, message string, cause error) {
	respond(logger, w, r, http.StatusBadRequest, meta_v1.StatusReasonBadRequest, message, cause)
}

func RespondWithInternalError(logger *zap.Logger, w http.ResponseWriter, r *http.Request, message string, cause error) {
	respond(logger, w, r, http.StatusInternalServerError, meta_v1.StatusReasonInternalError, message, cause)
}

func RespondWithForbiddenError(logger *zap.Logger, w http.ResponseWriter, r *http.Request, message string, cause error) {
	respond(logger, w, r, http.StatusForbidden, meta_v1.StatusReasonForbidden, message, cause)
}

func RespondWithUnauthorizedError(logger *zap.Logger, w http.ResponseWriter, r *http.Request, message string, cause error) {
	respond(logger, w, r, http.StatusUnauthorized, meta_v1.StatusReasonUnauthorized, message, cause)
}

func RespondWithNotFoundError(logger *zap.Logger, w http.ResponseWriter, r *http.Request, message string, cause error) {
	respond(logger, w, r, http.StatusNotFound, meta_v1.StatusReasonNotFound, message, cause)
}
