package auth

import (
	"github.com/pkg/errors"
)

const (
	errAuthInfoMissing           = "AuthInfo not found in the context"
	errAuthInfoUsernameIsMissing = "AuthInfo does not contain a username"
)

type InfoError struct {
	message string
	info    AggregatorUserInfo
}

func newAuthInfoError(message string, authInfo AggregatorUserInfo) error {
	return errors.WithStack(&InfoError{
		message: message,
		info:    authInfo,
	})
}

func (e *InfoError) AuthInfo() AggregatorUserInfo {
	return e.info
}

func (e *InfoError) Error() string {
	return e.message
}

func isErrAuthInfo(e error, message string) bool {
	if e == nil {
		return false
	}
	if v, ok := errors.Cause(e).(*InfoError); ok {
		return v.message == message
	}
	return false
}

func ErrAuthInfoMissing(authInfo AggregatorUserInfo) error {
	return newAuthInfoError(errAuthInfoMissing, authInfo)
}

func IsErrAuthInfoMissing(e error) bool {
	return isErrAuthInfo(e, errAuthInfoMissing)
}

func ErrAuthInfoUsernameIsMissing(authInfo AggregatorUserInfo) error {
	return newAuthInfoError(errAuthInfoUsernameIsMissing, authInfo)
}

func IsErrAuthInfoUsernameIsMissing(e error) bool {
	return isErrAuthInfo(e, errAuthInfoUsernameIsMissing)
}
