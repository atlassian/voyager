package logz

import (
	"github.com/atlassian/ctrl"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ControllerGk is a zap field used to identify logs coming from a specific controller
// or controller constructor. It includes logs that don't involve processing an
// object.
func ControllerGk(gk schema.GroupKind) zapcore.Field {
	return zap.Stringer("ctrl_gk", &gk)
}

// Object returns a zap field used to record ObjectName.
func Object(obj meta_v1.Object) zapcore.Field {
	return ObjectName(obj.GetName())
}

// ObjectName is a zap field to identify logs with the object name of a specific
// object being processed in the ResourceEventHandler or in the Controller.
func ObjectName(name string) zapcore.Field {
	return zap.String("obj_name", name)
}

// ObjectGk is a zap field to identify logs with the object gk of a specific
// object being processed in the ResourceEventHandler or in the Controller.
func ObjectGk(gk schema.GroupKind) zapcore.Field {
	return zap.Stringer("obj_gk", &gk)
}

// DelegateName is a zap field to identify logs of an object name that was processed in
// the ResourceEventHandler, that had a lookup or owner that was queued instead.
func DelegateName(name string) zapcore.Field {
	return zap.String("delegate_name", name)
}

// DelegateGk is a zap field to identify logs of an object gk that was processed in
// the ResourceEventHandler, that had a lookup or owner that was queued instead.
func DelegateGk(gk schema.GroupKind) zapcore.Field {
	return zap.Stringer("delegate_gk", &gk)
}

// Operation is a zap field used in ResourceEventHandler to identify the operation
// that the logs are being produced from.
func Operation(operation ctrl.Operation) zapcore.Field {
	return zap.Stringer("operation", operation)
}

func Namespace(obj meta_v1.Object) zapcore.Field {
	return NamespaceName(obj.GetNamespace())
}

func NamespaceName(namespace string) zapcore.Field {
	if namespace == "" {
		return zap.Skip()
	}
	return zap.String("namespace", namespace)
}

func Iteration(iteration uint32) zapcore.Field {
	return zap.Uint32("iter", iteration)
}
