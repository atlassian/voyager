package process

import (
	"fmt"
	"time"

	"github.com/atlassian/ctrl"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
)

type gvkQueueKey struct {
	gvk schema.GroupVersionKind
	ctrl.QueueKey
}

func (g *gvkQueueKey) String() string {
	return fmt.Sprintf("%s, Ns=%s, N=%s", g.gvk, g.Namespace, g.Name)
}

// workQueue is a type safe wrapper around workqueue.RateLimitingInterface.
type workQueue struct {
	// Objects that need to be synced.
	queue                   workqueue.RateLimitingInterface
	workDeduplicationPeriod time.Duration
}

func (q *workQueue) shutDown() {
	q.queue.ShutDown()
}

func (q *workQueue) get() (item gvkQueueKey, shutdown bool) {
	i, s := q.queue.Get()
	if s {
		return gvkQueueKey{}, true
	}
	return i.(gvkQueueKey), false
}

func (q *workQueue) done(item gvkQueueKey) {
	q.queue.Done(item)
}

func (q *workQueue) forget(item gvkQueueKey) {
	q.queue.Forget(item)
}

func (q *workQueue) numRequeues(item gvkQueueKey) int {
	return q.queue.NumRequeues(item)
}

func (q *workQueue) addRateLimited(item gvkQueueKey) {
	q.queue.AddRateLimited(item)
}

func (q *workQueue) newQueueForGvk(gvk schema.GroupVersionKind) *gvkQueue {
	return &gvkQueue{
		queue:                   q.queue,
		gvk:                     gvk,
		workDeduplicationPeriod: q.workDeduplicationPeriod,
	}
}

type gvkQueue struct {
	queue                   workqueue.RateLimitingInterface
	gvk                     schema.GroupVersionKind
	workDeduplicationPeriod time.Duration
}

func (q *gvkQueue) Add(item ctrl.QueueKey) {
	q.queue.AddAfter(gvkQueueKey{
		gvk:      q.gvk,
		QueueKey: item,
	}, q.workDeduplicationPeriod)
}
