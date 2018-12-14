package util

import (
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

var (
	envVarReplacer = regexp.MustCompile("[^A-Za-z0-9]+")
)

func EnvironmentVariableNameFor(s string) string {
	return strings.ToUpper(envVarReplacer.ReplaceAllString(s, "_"))
}

func EnvironmentVariablesAsMap(names ...string) (map[string]string, error) {
	vars := make(map[string]string, len(names))
	for _, name := range names {
		value, ok := os.LookupEnv(name)
		if !ok {
			return nil, errors.Errorf("could not find %q in environment variables", name)
		}
		vars[name] = value
	}
	return vars, nil
}
