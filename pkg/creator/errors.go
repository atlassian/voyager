package creator

import (
	"fmt"

	"github.com/pkg/errors"
)

// lackListError is for cases where a particular input has been reserved.
type BlackListError struct {
	message string
}

// NewBlackListError creates a BlackListError. blackListedClass is the the type of thing that is blacklisted, like a
// name or URL, and blackListedInstance is the actual instance that was inputted, like the name "voyager" or the URL
// "https://atlassian.com".
func NewBlackListError(blackListedClass string, blackListedInstance string) error {
	return errors.WithStack(&BlackListError{
		message: fmt.Sprintf("%q is reserved and can not be used for %q", blackListedInstance, blackListedClass),
	})
}

// Error is to implement the 'error' interface.
func (e *BlackListError) Error() string {
	return e.message
}

// IsBlackListError checks if 'e' is a BlackListError. Returns true when it is a BlackListError and is not nil.
func IsBlackListError(e error) bool {
	if e == nil {
		return false
	}
	_, ok := errors.Cause(e).(*BlackListError)
	return ok
}
