package monitor

import (
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	sdKey              = "sd"
	serviceKey         = "service"
	serviceInstanceKey = "si"
	ldKey              = "ld"
)

func ServiceDescriptor(sd *comp_v1.ServiceDescriptor) zapcore.Field {
	return zap.Any(sdKey, sd)
}

func Service(sd *creator_v1.Service) zapcore.Field {
	return zap.Any(serviceKey, sd)
}

func ServiceInstance(si *v1beta1.ServiceInstance) zapcore.Field {
	return zap.Any(serviceInstanceKey, si)
}

func LocationDescriptor(sd *form_v1.LocationDescriptor) zapcore.Field {
	return zap.Any(ldKey, sd)
}

func ServiceDescriptorError(err error) zapcore.Field {
	return zap.NamedError(sdKey, err)
}

func ServiceError(err error) zapcore.Field {
	return zap.NamedError(serviceKey, err)
}

func ServiceInstanceError(err error) zapcore.Field {
	return zap.NamedError(serviceInstanceKey, err)
}

func LocationDescriptorError(err error) zapcore.Field {
	return zap.NamedError(ldKey, err)
}
