package asapkey

import (
	"github.com/atlassian/voyager"
	core_v1 "k8s.io/api/core/v1"
)

func GetPublicKeyRepoEnvVars(location voyager.Location) []core_v1.EnvVar {
	var envDefault []core_v1.EnvVar
	switch {
	case location.EnvType == voyager.EnvTypeProduction:
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  RepositoryEnvVarName,
			Value: RepositoryProd,
		})
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  RepositoryFallbackEnvVarName,
			Value: RepositoryFallbackProd,
		})
	default:
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  RepositoryEnvVarName,
			Value: RepositoryStg,
		})
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  RepositoryFallbackEnvVarName,
			Value: RepositoryFallbackStg,
		})
	}
	return envDefault
}
