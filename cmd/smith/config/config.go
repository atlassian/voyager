package config

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/iamrole"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretenvvar/kubecompute"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretenvvar/microscompute"
	"github.com/atlassian/voyager/pkg/execution/plugins/secretparameter"
)

func Plugins() []smith_plugin.NewFunc {
	return []smith_plugin.NewFunc{
		iamrole.New,
		microscompute.New,
		kubecompute.New,
		secretparameter.New,
	}
}
