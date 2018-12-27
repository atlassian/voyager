package servicecentral

import (
	"fmt"

	"github.com/pkg/errors"
)

type errorCode string

type Error struct {
	code    errorCode
	message string
}

const (
	errorCodeNotFound errorCode = "NotFound"
)

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e *Error) is(code errorCode) bool {
	return e != nil && e.code == code
}

func cause(err error) *Error {
	if err == nil {
		return nil
	}

	cause := errors.Cause(err)
	if cause == nil {
		return nil
	}

	v, ok := cause.(*Error)
	if !ok {
		return nil
	}

	return v
}

func IsNotFound(err error) bool {
	return cause(err).is(errorCodeNotFound)
}

func NewNotFound(format string, v ...interface{}) error {
	return newError(errorCodeNotFound, format, v...)
}

func newError(code errorCode, message string, v ...interface{}) error {
	return errors.WithStack(&Error{
		code:    code,
		message: fmt.Sprintf(message, v...),
	})
}
