package svccatadmission

import (
	"strings"

	"github.com/atlassian/voyager"
)

func getServiceNameFromNamespace(namespace string) voyager.ServiceName {
	// This is a nasty hack for now so that the admission controller
	// doesn't need to talk to the cluster. It may break if we ever change how
	// this works (but we're unlikely to)... the good news is, it should break in a clean way
	// (the service probably won't exist, so we will fail to do anything here).
	// DO NOT replicate this approach in other parts of the codebase without discussion.
	return voyager.ServiceName(strings.Split(namespace, "--")[0])
}
