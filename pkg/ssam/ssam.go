package ssam

import (
	"fmt"
	"regexp"

	"github.com/atlassian/voyager"
	"github.com/pkg/errors"
)

var (
	regexpSSAMDelimiter = regexp.MustCompile("(-dl$|-dl-|^dl-)")
)

// ValidateServiceName validates that a specified service will not violate our requirements
// for naming of SSAM containers
func ValidateServiceName(serviceName voyager.ServiceName) error {
	index := regexpSSAMDelimiter.FindStringIndex(string(serviceName))

	if index == nil {
		return nil
	}

	return errors.Errorf("bad service name: cannot contain %q, found at index [%d, %d]",
		serviceName[index[0]:index[1]], index[0], index[1])
}

// TODO - rename, this isn't the access level name, it's an LDAP group prefix
// AccessLevelNameForEnvType determines the name of an access level based on the environment type
func AccessLevelNameForEnvType(ssamContainerName string, envType voyager.EnvType) string {
	return fmt.Sprintf("%s-dl-%s", ssamContainerName, string(envType))
}
