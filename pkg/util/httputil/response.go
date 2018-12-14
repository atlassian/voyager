package httputil

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func WriteResponse(logger *zap.Logger, w http.ResponseWriter, responseCode int, body []byte) {
	w.WriteHeader(responseCode)
	_, err := w.Write(body)
	if err != nil {
		logger.Warn(fmt.Sprintf("failed to write response body %v", truncatedString(body)), zap.Error(errors.WithStack(err)))
	}
}

// truncate to not pollute logs with too much information
func truncatedString(body []byte) string {
	if len(body) > 100 {
		return string(body[0:100])
	}
	return string(body)
}

func WriteOkResponse(logger *zap.Logger, w http.ResponseWriter, body []byte) {
	WriteResponse(logger, w, http.StatusOK, body)
}

func WriteNoContentResponse(logger *zap.Logger, w http.ResponseWriter) {
	WriteResponse(logger, w, http.StatusNoContent, []byte{})
}

func WriteIntenalServerErrorResponse(logger *zap.Logger, w http.ResponseWriter, err error) {
	WriteResponse(logger, w, http.StatusInternalServerError, []byte(err.Error()))
}
