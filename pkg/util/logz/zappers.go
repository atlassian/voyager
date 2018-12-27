package logz

import (
	"github.com/atlassian/voyager"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Account(account voyager.Account) zap.Field {
	return zap.String("account", string(account))
}

func EnvType(envType voyager.EnvType) zap.Field {
	return zap.String("env_type", string(envType))
}

func Label(label voyager.Label) zap.Field {
	return zap.String("label", string(label))
}

func Region(region voyager.Region) zap.Field {
	return zap.String("region", string(region))
}

func ServiceName(service voyager.ServiceName) zap.Field {
	return ServiceNameString(string(service))
}

func ServiceNameString(service string) zap.Field {
	return zap.String("service_name", service)
}

func Namespace(obj meta_v1.Object) zapcore.Field {
	return NamespaceName(obj.GetNamespace())
}

func NamespaceName(name string) zapcore.Field {
	if name == "" {
		return zap.Skip()
	}
	return zap.String("request_namespace", name)
}
