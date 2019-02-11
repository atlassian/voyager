package process

import (
	"context"
	"net/http"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	"github.com/atlassian/ctrl/logz"
	chimw "github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// Work queue deduplicates scheduled keys. This is the period it waits for duplicate keys before letting the work
	// to be dequeued.
	workDeduplicationPeriod = 50 * time.Millisecond

	metricsNamespace = "ctrl"
)

type Generic struct {
	iter        uint32
	logger      *zap.Logger
	queue       workQueue
	workers     uint
	Controllers map[schema.GroupVersionKind]Holder
	Servers     map[schema.GroupVersionKind]ServerHolder
	Informers   map[schema.GroupVersionKind]cache.SharedIndexInformer
}

func NewGeneric(config *ctrl.Config, queue workqueue.RateLimitingInterface, workers uint, constructors ...ctrl.Constructor) (*Generic, error) {
	controllers := make(map[schema.GroupVersionKind]ctrl.Interface)
	servers := make(map[schema.GroupVersionKind]ctrl.Server)
	holders := make(map[schema.GroupVersionKind]Holder)
	informers := make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	serverHolders := make(map[schema.GroupVersionKind]ServerHolder)
	wq := workQueue{
		queue:                   queue,
		workDeduplicationPeriod: workDeduplicationPeriod,
	}
	for _, constr := range constructors {
		descr := constr.Describe()

		readyForWork := make(chan struct{})
		queueGvk := wq.newQueueForGvk(descr.Gvk)
		groupKind := descr.Gvk.GroupKind()
		controllerLogger := config.Logger
		constructorConfig := config
		constructorConfig.Logger = controllerLogger

		// Extra api data
		requestTime := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Name:      "request_time_seconds",
				Help:      "Number of seconds each request takes to the controller provided server",
			},
			[]string{"url", "method", "status", "controller", "groupkind"},
		)

		var allMetrics []prometheus.Collector

		constructed, err := constr.New(
			constructorConfig,
			&ctrl.Context{
				ReadyForWork: func() {
					close(readyForWork)
				},
				Middleware:  addMetricsMiddleware(requestTime, config.AppName, groupKind.String()),
				Informers:   informers,
				Controllers: controllers,
				WorkQueue:   queueGvk,
			},
		)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to construct controller for GVK %s", descr.Gvk)
		}

		if constructed.Interface == nil && constructed.Server == nil {
			return nil, errors.Wrapf(err, "failed to construct controller or server for GVK %s", descr.Gvk)
		}

		if constructed.Interface != nil {
			if _, ok := controllers[descr.Gvk]; ok {
				return nil, errors.Errorf("duplicate controller for GVK %s", descr.Gvk)
			}
			inf, ok := informers[descr.Gvk]
			if !ok {
				return nil, errors.Errorf("controller for GVK %s should have registered an informer for that GVK", descr.Gvk)
			}
			inf.AddEventHandler(&handlers.GenericHandler{
				Logger:    controllerLogger,
				WorkQueue: queueGvk,
				Gvk:       descr.Gvk,
			})

			controllers[descr.Gvk] = constructed.Interface

			objectProcessTime := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Namespace: metricsNamespace,
					Name:      "process_object_seconds",
					Help:      "Histogram measuring the time it took to process an object",
				},
				[]string{"controller", "object_namespace", "object", "groupkind"},
			)
			objectProcessErrors := prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: metricsNamespace,
					Name:      "process_object_errors_total",
					Help:      "Records the number of times an error was triggered while processing an object",
				},
				[]string{"controller", "object_namespace", "object", "groupkind", "retriable"},
			)

			holders[descr.Gvk] = Holder{
				AppName:             config.AppName,
				Cntrlr:              constructed.Interface,
				ReadyForWork:        readyForWork,
				objectProcessTime:   objectProcessTime,
				objectProcessErrors: objectProcessErrors,
			}

			allMetrics = append(allMetrics, objectProcessTime, objectProcessErrors)
		}

		if constructed.Server != nil {
			if _, ok := servers[descr.Gvk]; ok {
				return nil, errors.Errorf("duplicate server for GVK %s", descr.Gvk)
			}
			servers[descr.Gvk] = constructed.Server

			serverHolders[descr.Gvk] = ServerHolder{
				AppName:      config.AppName,
				Server:       constructed.Server,
				ReadyForWork: readyForWork,
				requestTime:  requestTime,
			}

			allMetrics = append(allMetrics, requestTime)
		}

		for _, metric := range allMetrics {
			if err := constructorConfig.Registry.Register(metric); err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}

	return &Generic{
		logger:      config.Logger,
		queue:       wq,
		workers:     workers,
		Controllers: holders,
		Servers:     serverHolders,
		Informers:   informers,
	}, nil
}

func (g *Generic) Run(ctx context.Context) error {
	// Stager will perform ordered, graceful shutdown
	stgr := stager.New()
	defer stgr.Shutdown()
	defer g.queue.shutDown()

	// Stage: start all informers then wait on them
	stage := stgr.NextStage()
	for _, inf := range g.Informers {
		inf := inf // capture field into a scoped variable to avoid data race
		stage.StartWithChannel(func(stopCh <-chan struct{}) {
			defer logz.LogStructuredPanic()
			inf.Run(stopCh)
		})
	}
	g.logger.Info("Waiting for informers to sync")
	for _, inf := range g.Informers {
		if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
			return ctx.Err()
		}
	}
	g.logger.Info("Informers synced")

	// Stage: start all controllers then wait for them to signal ready for work
	stage = stgr.NextStage()
	for _, c := range g.Controllers {
		c := c // capture field into a scoped variable to avoid data race
		stage.StartWithContext(func(ctx context.Context) {
			defer logz.LogStructuredPanic()
			c.Cntrlr.Run(ctx)
		})
	}
	for gvk, c := range g.Controllers {
		select {
		case <-ctx.Done():
			g.logger.Sugar().Infof("Was waiting for the controller for %s to become ready for processing", gvk)
			return ctx.Err()
		case <-c.ReadyForWork:
		}
	}

	// Stage: start workers
	stage = stgr.NextStage()
	for i := uint(0); i < g.workers; i++ {
		stage.Start(func() {
			defer logz.LogStructuredPanic()
			g.worker()
		})
	}

	if len(g.Servers) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	// Stage: start servers
	group, ctx := errgroup.WithContext(ctx)
	for _, srv := range g.Servers {
		server := srv.Server // capture field into a scoped variable to avoid data race
		group.Go(func() error {
			return server.Run(ctx)
		})
	}
	return group.Wait()
}

func (g *Generic) IsReady() bool {
	for _, holder := range g.Controllers {
		select {
		case <-holder.ReadyForWork:
		default:
			return false
		}
	}
	for _, holder := range g.Servers {
		select {
		case <-holder.ReadyForWork:
		default:
			return false
		}
	}
	return true
}

func addMetricsMiddleware(requestTime *prometheus.HistogramVec, controller, groupKind string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			t0 := time.Now()
			next.ServeHTTP(res, r)
			tn := time.Since(t0)

			requestTime.WithLabelValues(r.URL.Path, r.Method, string(res.Status()), controller, groupKind).Observe(tn.Seconds())
		})
	}
}

type Holder struct {
	AppName             string
	Cntrlr              ctrl.Interface
	ReadyForWork        <-chan struct{}
	objectProcessTime   *prometheus.HistogramVec
	objectProcessErrors *prometheus.CounterVec
}

type ServerHolder struct {
	AppName      string
	Server       ctrl.Server
	ReadyForWork <-chan struct{}
	requestTime  *prometheus.HistogramVec
}
