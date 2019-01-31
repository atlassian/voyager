package updater

import (
	"reflect"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type ObjectUpdater struct {
	ExistingObjectsIndexer cache.Indexer
	Client                 Client
	SpecCheck              SpecCheck
}

type Client interface {
	Create(ns string, obj runtime.Object) (runtime.Object, error)
	Update(ns string, obj runtime.Object) (runtime.Object, error)
	Delete(ns string, name string, options *meta_v1.DeleteOptions) error
}

type ClientAdapter struct {
	CreateMethod func(ns string, obj runtime.Object) (runtime.Object, error)
	UpdateMethod func(ns string, obj runtime.Object) (runtime.Object, error)
	DeleteMethod func(ns string, name string, options *meta_v1.DeleteOptions) error
}

func (p ClientAdapter) Create(ns string, obj runtime.Object) (runtime.Object, error) {
	return p.CreateMethod(ns, obj)
}

func (p ClientAdapter) Update(ns string, obj runtime.Object) (runtime.Object, error) {
	return p.UpdateMethod(ns, obj)
}

func (p ClientAdapter) Delete(ns string, name string, options *meta_v1.DeleteOptions) error {
	return p.DeleteMethod(ns, name, options)
}

// CreateOrUpdate given the object to create/update, and the owner of the object
// will attempt to create or update the object, only if the owner matches the
// object. In the case of updates, it will only update if the object has changed.
// This returns one of four possible exclusive states:
// 1. Conflict - object in k8s conflicts with update or create. Conflict is true.
// 2. Precondition Failed Error - Precondition failed. Error is copied from updatePrecondition function.
// 3. Error with Retriable flag - An error happened, and if it is retriable.
// 4. Success - Successfully created or updated the object. It is returned.
func (o *ObjectUpdater) CreateOrUpdate(logger *zap.Logger, updatePrecondition func(runtime.Object) error, obj runtime.Object) (conflictRet, retriableRet bool, result runtime.Object, err error) {
	existingObj, exists, err := o.ExistingObjectsIndexer.Get(obj)
	if err != nil {
		return false, false, nil, err
	}
	if exists {
		existingCopy := existingObj.(runtime.Object).DeepCopyObject()
		existingCopy.GetObjectKind().SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

		if preconditionErr := updatePrecondition(existingCopy); preconditionErr != nil {
			// Preconditions does not pass
			return false, false, nil, preconditionErr
		}

		return o.Update(logger, obj, existingCopy)
	}
	return o.Create(logger, obj)
}

func pointerToNewObjectWithTypeOf(original runtime.Object) (runtime.Object, error) {
	t := reflect.TypeOf(original)
	value := reflect.ValueOf(original)
	if t.Kind() != reflect.Ptr || value.IsNil() {
		return nil, errors.Errorf("update requires a non-nil pointer to an object, got %v", t)
	}

	return reflect.New(t.Elem()).Interface().(runtime.Object), nil
}

func (o *ObjectUpdater) Update(logger *zap.Logger, desired, existing runtime.Object) (conflictRet, retriableRet bool, result runtime.Object, err error) {
	logger.Info("Updating object")
	kind := existing.GetObjectKind().GroupVersionKind().String()
	meta := existing.(meta_v1.Object)

	updated, match, _, err := o.SpecCheck.CompareActualVsSpec(logger, desired, existing)
	if err != nil {
		return false, false, nil, errors.Wrapf(err, "error comparing spec and actual %s", kind)
	}
	if match {
		logger.Info("Object exists and is up to date")
		return false, false, existing, nil
	}

	newObj, err := pointerToNewObjectWithTypeOf(desired)
	if err != nil {
		return false, false, nil, err
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(updated.Object, newObj)
	if err != nil {
		return false, false, nil, err
	}

	newMeta := newObj.(meta_v1.Object)
	logger.Sugar().Infof("Updating existing object for %q", newMeta.GetName())

	result, err = o.Client.Update(meta.GetNamespace(), newObj)
	if err != nil {
		if api_errors.IsConflict(err) {
			return true, false, nil, err
		}
		if api_errors.IsInvalid(err) {
			return false, true, nil, err
		}
		return false, true, nil, err
	}
	return false, false, result, nil
}

func (o *ObjectUpdater) Create(logger *zap.Logger, obj runtime.Object) (conflictRet, retriableRet bool, result runtime.Object, err error) {
	logger.Info("Creating object")

	meta := obj.(meta_v1.Object)

	result, err = o.Client.Create(meta.GetNamespace(), obj)
	if err != nil {
		if api_errors.IsAlreadyExists(err) {
			return true, false, nil, err
		}
		if api_errors.IsInvalid(err) {
			return false, true, nil, err
		}
		return false, true, nil, err
	}
	return false, false, result, nil
}

func (o *ObjectUpdater) Delete(logger *zap.Logger, ns, name string, options *meta_v1.DeleteOptions) (conflictRet, retriableRet bool, err error) {
	logger.Info("Deleting object")

	err = o.Client.Delete(ns, name, options)
	if err != nil {
		if api_errors.IsNotFound(err) {
			return false, false, nil
		}
		if api_errors.IsConflict(err) {
			return true, false, err
		}
		return false, true, err
	}
	return false, false, nil
}

func (o *ObjectUpdater) DeleteAndGet(logger *zap.Logger, ns, name string) (conflictRet, retriableRet bool, obj runtime.Object, err error) {
	existing, exists, err := o.Get(logger, ns, name)
	if err != nil {
		return false, true, nil, err
	}
	if !exists {
		return false, false, nil, nil
	}

	existingMeta := existing.(meta_v1.ObjectMetaAccessor)
	if existingMeta.GetObjectMeta().GetDeletionTimestamp() != nil {
		// Already marked for deletion
		return false, false, existing, nil
	}

	foreground := meta_v1.DeletePropagationForeground
	deleteOptions := meta_v1.DeleteOptions{
		PropagationPolicy: &foreground,
	}
	conflict, retriable, err := o.Delete(logger, ns, name, &deleteOptions)
	if err != nil {
		return conflict, retriable, nil, err
	}

	existing, exists, err = o.Get(logger, ns, name)
	if err != nil {
		return false, true, nil, err
	}
	if !exists {
		return false, false, nil, nil
	}

	return false, false, existing, nil
}

func (o *ObjectUpdater) Get(logger *zap.Logger, ns, name string) (result runtime.Object, exists bool, err error) {
	logger.Info("Getting object")

	existingObj, exists, err := o.ExistingObjectsIndexer.GetByKey(getObjectKey(ns, name))
	if err != nil || !exists {
		return nil, exists, err
	}
	existingCopy := existingObj.(runtime.Object).DeepCopyObject()
	return existingCopy, exists, err
}

func getObjectKey(namespace string, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}
