package util

import (
	"fmt"
)

type recoverableError interface {
	canRecover() bool
}

func CanRecover(errorList ...error) bool {
	for _, err := range errorList {
		r, ok := err.(recoverableError)
		if !ok || !r.canRecover() {
			return false
		}
	}

	return true
}

type ErrInvalidVariableName struct {
	varName string
}

func NewErrInvalidVariableName(varName string) *ErrInvalidVariableName {
	return &ErrInvalidVariableName{
		varName: varName,
	}
}

func (e *ErrInvalidVariableName) Error() string {
	return fmt.Sprintf("invalid variable name: %s", e.varName)
}

func (e *ErrInvalidVariableName) canRecover() bool {
	return true
}

type ErrVariableNotFound struct {
	varName string
	Similar string
}

func NewErrVariableNotFound(varName string, similar string) *ErrVariableNotFound {
	return &ErrVariableNotFound{
		varName: varName,
		Similar: similar,
	}
}

func (e *ErrVariableNotFound) Error() string {
	if e.Similar == "" {
		return fmt.Sprintf("variable not defined: %q", e.varName)
	}
	return fmt.Sprintf("variable not defined: %q, did you mean %q", e.varName, e.Similar)
}

func (e *ErrVariableNotFound) canRecover() bool {
	return true
}

type ErrorList struct {
	ErrorList []error
}

func NewErrorList(initialErrList ...error) *ErrorList {
	errorList := append([]error{}, initialErrList...)

	return &ErrorList{
		ErrorList: errorList,
	}
}

func (e *ErrorList) Add(err ...error) {
	for _, currErr := range err {
		switch errList := currErr.(type) {
		case *ErrorList:
			e.AddErrorList(errList)
		default:
			e.ErrorList = append(e.ErrorList, currErr)
		}
	}
}

func (e *ErrorList) AddErrorList(errorList *ErrorList) {
	e.ErrorList = append(e.ErrorList, errorList.ErrorList...)
}

func (e *ErrorList) Error() string {
	if len(e.ErrorList) == 0 {
		return ""
	}

	result := e.ErrorList[0].Error()

	for i := 1; i < len(e.ErrorList); i++ {
		result += fmt.Sprintf(", %s", e.ErrorList[i].Error())
	}

	return result
}

func (e *ErrorList) CanRecover() bool {
	return CanRecover(e.ErrorList...)
}

func (e *ErrorList) HasErrors() bool {
	return len(e.ErrorList) > 0
}
