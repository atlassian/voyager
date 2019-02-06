package compute

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	core_v1 "k8s.io/api/core/v1"
)

func GetSharedDefaultEnvVars(location voyager.Location) []core_v1.EnvVar {
	var envDefault []core_v1.EnvVar
	switch {
	case location.EnvType == voyager.EnvTypeProduction:
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  asapkey.RepositoryEnvVarName,
			Value: asapkey.RepositoryProd,
		})
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  asapkey.RepositoryFallbackEnvVarName,
			Value: asapkey.RepositoryFallbackProd,
		})
	default:
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  asapkey.RepositoryEnvVarName,
			Value: asapkey.RepositoryStg,
		})
		envDefault = append(envDefault, core_v1.EnvVar{
			Name:  asapkey.RepositoryFallbackEnvVarName,
			Value: asapkey.RepositoryFallbackStg,
		})
	}
	return envDefault
}
