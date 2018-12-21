package handlers

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// LookupHandler is a handler for controlled objects that can be mapped to some controller object
// through the use of a Lookup function.
// This handler assumes that the Logger already has the ctrl_gk field set.
type LookupHandler struct {
	Logger    *zap.Logger
	WorkQueue ctrl.WorkQueueProducer
	Gvk       schema.GroupVersionKind

	Lookup func(runtime.Object) ([]runtime.Object, error)
}

func (e *LookupHandler) enqueueMapped(logger *zap.Logger, obj meta_v1.Object) {
	logger = e.loggerForObj(logger, obj)
	objs, err := e.Lookup(obj.(runtime.Object))
	if err != nil {
		logger.Error("Failed to lookup objects", zap.Error(err))
		return
	}
	if len(objs) == 0 {
		logger.Debug("Lookup function returned zero results")
	}
	for _, o := range objs {
		metaobj := o.(meta_v1.Object)
		logger.
			With(logz.DelegateName(obj.GetName())).
			With(logz.DelegateGk(e.Gvk.GroupKind())).
			Info("Enqueuing looked up object")
		e.WorkQueue.Add(ctrl.QueueKey{
			Namespace: metaobj.GetNamespace(),
			Name:      metaobj.GetName(),
		})
	}
}

func (e *LookupHandler) OnAdd(obj interface{}) {
	logger := e.Logger.With(logz.Operation(ctrl.AddedOperation))
	e.enqueueMapped(logger, obj.(meta_v1.Object))
}

func (e *LookupHandler) OnUpdate(oldObj, newObj interface{}) {
	logger := e.Logger.With(logz.Operation(ctrl.UpdatedOperation))
	e.enqueueMapped(logger, newObj.(meta_v1.Object))
}

func (e *LookupHandler) OnDelete(obj interface{}) {
	metaObj, ok := obj.(meta_v1.Object)
	logger := e.Logger.With(logz.Operation(ctrl.DeletedOperation))
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		metaObj, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			logger.Sugar().Errorf("Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	e.enqueueMapped(logger, metaObj)
}

// loggerForObj returns a logger with fields for a controlled object.
func (e *LookupHandler) loggerForObj(logger *zap.Logger, obj meta_v1.Object) *zap.Logger {
	return logger.With(logz.Namespace(obj),
		logz.Object(obj),
		logz.ObjectGk(e.Gvk.GroupKind()))
}
