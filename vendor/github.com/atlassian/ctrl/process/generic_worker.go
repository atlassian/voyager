package process

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	// maxRetries is the number of times an object will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// an object is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

func (g *Generic) worker() {
	for g.processNextWorkItem() {
	}
}

func (g *Generic) processNextWorkItem() bool {
	key, quit := g.queue.get()
	if quit {
		return false
	}
	defer g.queue.done(key)

	holder := g.Controllers[key.gvk]
	logger := g.logger.With(logz.NamespaceName(key.Namespace),
		logz.ObjectName(key.Name),
		logz.ObjectGk(key.gvk.GroupKind()),
		logz.Iteration(atomic.AddUint32(&g.iter, 1)))

	external, retriable, err := g.processKey(logger, holder, key)
	g.handleErr(logger, holder, external, retriable, err, key)

	return true
}

func (g *Generic) handleErr(logger *zap.Logger, holder Holder, external bool, retriable bool, err error, key gvkQueueKey) {
	groupKind := key.gvk.GroupKind()

	if err == nil {
		g.queue.forget(key)
		return
	}

	if retriable && g.queue.numRequeues(key) < maxRetries {
		logger.Info("Error syncing object, will retry", zap.Error(err))
		g.queue.addRateLimited(key)
		holder.objectProcessErrors.
			WithLabelValues(holder.AppName, key.Namespace, key.Name, groupKind.String(), strconv.FormatBool(external), strconv.FormatBool(true)).
			Inc()
		return
	}

	holder.objectProcessErrors.
		WithLabelValues(holder.AppName, key.Namespace, key.Name, groupKind.String(), strconv.FormatBool(external), strconv.FormatBool(false)).
		Inc()

	if external {
		logger.Info("Dropping object out of the queue due to external error", zap.Error(err))
	} else {
		logger.Error("Dropping object out of the queue due to internal error", zap.Error(err))
	}
	g.queue.forget(key)
}

func (g *Generic) processKey(logger *zap.Logger, holder Holder, key gvkQueueKey) (bool /* external */, bool /*retriable*/, error) {
	groupKind := key.gvk.GroupKind()

	cntrlr := holder.Cntrlr
	informer := g.Informers[key.gvk]
	obj, exists, err := getFromIndexer(informer.GetIndexer(), key.gvk, key.Namespace, key.Name)
	if err != nil {
		return false, false, errors.Wrapf(err, "failed to get object by key %s", key.String())
	}
	if !exists {
		logger.Debug("Object not in cache. Was deleted?")
		return false, false, nil
	}
	startTime := time.Now()
	logger.Info("Started syncing")

	msg := ""
	defer func() {
		totalTime := time.Since(startTime)
		holder.objectProcessTime.WithLabelValues(holder.AppName, key.Namespace, key.Name, groupKind.String()).Observe(totalTime.Seconds())
		logger.Sugar().Infof("Synced in %v%s", totalTime, msg)
	}()

	external, retriable, err := cntrlr.Process(&ctrl.ProcessContext{
		Logger: logger,
		Object: obj,
	})
	if err != nil && api_errors.IsConflict(errors.Cause(err)) {
		msg = " (conflict)"
		err = nil
	}
	return external, retriable, err
}

func getFromIndexer(indexer cache.Indexer, gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, bool /*exists */, error) {
	obj, exists, err := indexer.GetByKey(ByNamespaceAndNameIndexKey(namespace, name))
	if err != nil || !exists {
		return nil, exists, err
	}
	ro := obj.(runtime.Object).DeepCopyObject()
	ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
	return ro, true, nil
}

func ByNamespaceAndNameIndexKey(namespace, name string) string {
	if namespace == meta_v1.NamespaceNone {
		return name
	}
	return namespace + "/" + name
}
