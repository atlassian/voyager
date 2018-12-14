package httputil

import (
	"fmt"

	"github.com/pkg/errors"
)

type ClientError struct {
	code    errorCode
	message string
}

type errorCode string

const (
	errorCodeBadRequest  errorCode = "BadRequest"
	errorCodeConflict    errorCode = "Conflict"
	errorCodeNotFound    errorCode = "NotFound"
	errorCodeServerError errorCode = "ServerError"
	errorCodeUnknown     errorCode = "Unknown"
)

func (c *ClientError) Error() string {
	return fmt.Sprintf("%s: %s", c.code, c.message)
}

func cause(err error) *ClientError {
	if err == nil {
		return nil
	}

	cause := errors.Cause(err)
	if cause == nil {
		return nil
	}

	v, ok := cause.(*ClientError)
	if !ok {
		return nil
	}

	return v
}

func IsBadRequest(err error) bool {
	return cause(err).is(errorCodeBadRequest)
}

func IsConflict(err error) bool {
	return cause(err).is(errorCodeConflict)
}

func IsNotFound(err error) bool {
	return cause(err).is(errorCodeNotFound)
}

func IsServerError(err error) bool {
	return cause(err).is(errorCodeServerError)
}

func IsUnknown(err error) bool {
	return cause(err).is(errorCodeUnknown)
}

func (c *ClientError) is(code errorCode) bool {
	return c != nil && c.code == code
}

func NewBadRequest(format string, v ...interface{}) error {
	return newError(errorCodeBadRequest, format, v...)
}

func NewConflict(format string, v ...interface{}) error {
	return newError(errorCodeConflict, format, v...)
}

func NewNotFound(format string, v ...interface{}) error {
	return newError(errorCodeNotFound, format, v...)
}

func NewServerError(code int, format string, v ...interface{}) error {
	return newError(errorCodeServerError, fmt.Sprintf("error processing response with statuscode %d: %s", code, format), v...)
}

func NewUnknown(format string, v ...interface{}) error {
	return newError(errorCodeUnknown, format, v...)
}

func newError(code errorCode, message string, v ...interface{}) error {
	return errors.WithStack(&ClientError{
		code:    code,
		message: fmt.Sprintf(message, v...),
	})
}
