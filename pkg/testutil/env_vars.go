package testutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
)

func ParseEnvFile(t *testing.T, file string) {
	doc, err := ioutil.ReadFile(file)
	require.NoError(t, err)

	var envVars map[string]string
	err = yaml.Unmarshal(doc, &envVars)
	require.NoError(t, err)

	for k, v := range envVars {
		err = os.Setenv(k, v)
		require.NoError(t, err)
	}
}
