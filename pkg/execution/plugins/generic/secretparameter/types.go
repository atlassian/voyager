package secretparameter

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
)

type Spec struct {
	// double map should be read as:
	//   map the secret contents of a particular smith resource into the SC parameter format
	//   according to the map[string]string, where LHS original naming -> RHS parameter naming
	//   If a secret key is not in the map, it is ignored.
	// See README.md for an example.
	Mapping map[smith_v1.ResourceName]map[string]string `json:"mapping"`
}
