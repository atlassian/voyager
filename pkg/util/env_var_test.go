package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvVarNoop(t *testing.T) {
	t.Parallel()

	s := "ENV_VAR"
	assert.Equal(t, s, EnvironmentVariableNameFor(s))
}

func TestEnvVarDash(t *testing.T) {
	t.Parallel()

	s := "ENV-VAR"
	assert.Equal(t, "ENV_VAR", EnvironmentVariableNameFor(s))
}

func TestEnvVarLowercase(t *testing.T) {
	t.Parallel()

	s := "env-var"
	assert.Equal(t, "ENV_VAR", EnvironmentVariableNameFor(s))
}

func TestEnvVarDashWhitespace(t *testing.T) {
	t.Parallel()

	s := " ENV VAR "
	assert.Equal(t, "_ENV_VAR_", EnvironmentVariableNameFor(s))
}

func TestEnvVarSymbols(t *testing.T) {
	t.Parallel()

	s := "$#ENV@%VAR*("
	assert.Equal(t, "_ENV_VAR_", EnvironmentVariableNameFor(s))
}

func TestEnvVarInvalidString(t *testing.T) {
	t.Parallel()

	s := "Привет мир!"
	assert.Equal(t, "_", EnvironmentVariableNameFor(s))
}
