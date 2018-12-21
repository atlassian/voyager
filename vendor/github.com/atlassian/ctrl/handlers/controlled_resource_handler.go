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

// ControllerIndex is an index from controlled to controller objects.
type ControllerIndex interface {
	// ControllerByObject returns controller objects that own or want to own an object with a particular Group, Kind,
	// namespace and name. "want to own" means that the object might not exist yet but the controller
	// object would want it to.
	ControllerByObject(gk schema.GroupKind, namespace, name string) ([]runtime.Object, error)
}

// ControlledResourceHandler is a handler for objects the are controlled/owned/produced by some controller object.
// The controller object is identified by a controller owner reference on the controlled objects.
// This handler assumes that:
// - Logger already has the cntrl_gk field set.
// - controlled and controller objects exist in the same namespace and never across namespaces.
type ControlledResourceHandler struct {
	Logger          *zap.Logger
	WorkQueue       ctrl.WorkQueueProducer
	ControllerIndex ControllerIndex
	ControllerGvk   schema.GroupVersionKind
	Gvk             schema.GroupVersionKind
}

func (g *ControlledResourceHandler) enqueueMapped(logger *zap.Logger, metaObj meta_v1.Object) {
	name, namespace := g.getControllerNameAndNamespace(metaObj)
	logger = g.loggerForObj(logger, metaObj)

	if name == "" {
		if g.ControllerIndex != nil {
			controllers, err := g.ControllerIndex.ControllerByObject(
				metaObj.(runtime.Object).GetObjectKind().GroupVersionKind().GroupKind(), namespace, metaObj.GetName())
			if err != nil {
				logger.Error("Failed to get controllers for object", zap.Error(err))
				return
			}
			for _, controller := range controllers {
				controllerMeta := controller.(meta_v1.Object)
				g.rebuildControllerByName(logger, controllerMeta.GetNamespace(), controllerMeta.GetName())
			}
		}
	} else {
		g.rebuildControllerByName(logger, namespace, name)
	}
}

func (g *ControlledResourceHandler) OnAdd(obj interface{}) {
	metaObj := obj.(meta_v1.Object)
	logger := g.Logger.With(logz.Operation(ctrl.AddedOperation))
	g.enqueueMapped(logger, metaObj)
}

func (g *ControlledResourceHandler) OnUpdate(oldObj, newObj interface{}) {
	oldMeta := oldObj.(meta_v1.Object)
	newMeta := newObj.(meta_v1.Object)
	logger := g.Logger.With(logz.Operation(ctrl.UpdatedOperation))

	oldName, _ := g.getControllerNameAndNamespace(oldMeta)
	newName, _ := g.getControllerNameAndNamespace(newMeta)

	if oldName != newName {
		g.enqueueMapped(logger, oldMeta)
	}

	g.enqueueMapped(logger, newMeta)
}

func (g *ControlledResourceHandler) OnDelete(obj interface{}) {
	metaObj, ok := obj.(meta_v1.Object)
	logger := g.Logger.With(logz.Operation(ctrl.DeletedOperation))
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
	g.enqueueMapped(logger, metaObj)
}

// This method may be called with an empty controllerName.
func (g *ControlledResourceHandler) rebuildControllerByName(logger *zap.Logger, namespace, controllerName string) {
	if controllerName == "" {
		logger.Debug("Object has no controller, so nothing was enqueued")
		return
	}
	logger.
		With(logz.DelegateName(controllerName)).
		With(logz.DelegateGk(g.ControllerGvk.GroupKind())).
		Info("Enqueuing controller")
	g.WorkQueue.Add(ctrl.QueueKey{
		Namespace: namespace,
		Name:      controllerName,
	})
}

// getControllerNameAndNamespace returns name and namespace of the object's controller.
// Returned name may be empty if the object does not have a controller owner reference.
func (g *ControlledResourceHandler) getControllerNameAndNamespace(obj meta_v1.Object) (string, string) {
	var name string
	ref := meta_v1.GetControllerOf(obj)
	if ref != nil && ref.APIVersion == g.ControllerGvk.GroupVersion().String() && ref.Kind == g.ControllerGvk.Kind {
		name = ref.Name
	}
	return name, obj.GetNamespace()
}

func (g *ControlledResourceHandler) loggerForObj(logger *zap.Logger, obj meta_v1.Object) *zap.Logger {
	return logger.With(logz.Namespace(obj),
		logz.Object(obj),
		logz.ObjectGk(g.Gvk.GroupKind()))
}
