package util

var (
	_ error = &ErrorList{}
	_ error = &ErrInvalidVariableName{}
	_ error = &ErrVariableNotFound{}
)
