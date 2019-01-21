package config

import (
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/iamrole"
	"github.com/atlassian/voyager/pkg/execution/plugins/generic/secretparameter"
	"github.com/atlassian/voyager/pkg/execution/plugins/generic/secretplugin"
)

func Plugins() []smith_plugin.NewFunc {
	return []smith_plugin.NewFunc{
		iamrole.New,
		secretparameter.New,
		secretplugin.New,
	}
}
