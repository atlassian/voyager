package zappers

import (
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func ProviderName(name string) zapcore.Field {
	return zap.String("provider_name", name)
}

func Route(route *ops_v1.Route) zapcore.Field {
	return ProviderName(route.Name)
}
