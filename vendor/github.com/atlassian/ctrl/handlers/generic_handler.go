package handlers

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// This handler assumes that the Logger already has the obj_gk/ctrl_gk field set.
type GenericHandler struct {
	Logger    *zap.Logger
	WorkQueue ctrl.WorkQueueProducer

	Gvk schema.GroupVersionKind
}

func (g *GenericHandler) OnAdd(obj interface{}) {
	logger := g.Logger.With(logz.Operation("added"))
	g.add(logger, obj.(meta_v1.Object))
}

func (g *GenericHandler) OnUpdate(oldObj, newObj interface{}) {
	logger := g.Logger.With(logz.Operation("updated"))
	g.add(logger, newObj.(meta_v1.Object))
}

func (g *GenericHandler) OnDelete(obj interface{}) {
	metaObj, ok := obj.(meta_v1.Object)
	logger := g.Logger.With(logz.Operation("deleted"))
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
	g.add(logger, metaObj)
}

func (g *GenericHandler) add(logger *zap.Logger, obj meta_v1.Object) {
	g.loggerForObj(logger, obj).Info("Enqueuing object")
	g.WorkQueue.Add(ctrl.QueueKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	})
}

func (g *GenericHandler) loggerForObj(logger *zap.Logger, obj meta_v1.Object) *zap.Logger {
	return logger.With(logz.Namespace(obj),
		logz.Object(obj),
		logz.ObjectGk(g.Gvk.GroupKind()))
}
