package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/flagutil"
	"github.com/atlassian/ctrl/logz"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/ctrl/process"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	core_v1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	defaultAuxServerAddr = ":9090"
)

type PrometheusRegistry interface {
	prometheus.Registerer
	prometheus.Gatherer
}

type App struct {
	Logger *zap.Logger

	options.GenericNamespacedControllerOptions
	options.LeaderElectionOptions
	options.RestClientOptions
	options.LoggerOptions

	MainClient         kubernetes.Interface
	PrometheusRegistry PrometheusRegistry

	// Name is the name of the application. It must only contain alphanumeric
	// characters.
	Name        string
	RestConfig  *rest.Config
	Controllers []ctrl.Constructor
	AuxListenOn string
	Debug       bool
}

func (a *App) Run(ctx context.Context) (retErr error) {
	defer func() {
		if err := a.Logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to flush (AKA sync) remaining logs: %v\n", err) // nolint: errcheck
		}
	}()

	// Controller
	config := &ctrl.Config{
		AppName:      a.Name,
		Namespace:    a.Namespace,
		ResyncPeriod: a.ResyncPeriod,
		Registry:     a.PrometheusRegistry,
		Logger:       a.Logger,

		RestConfig: a.RestConfig,
		MainClient: a.MainClient,
	}
	generic, err := process.NewGeneric(config,
		workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "multiqueue"),
		a.Workers, a.Controllers...)
	if err != nil {
		return err
	}

	// Auxiliary server
	auxSrv := AuxServer{
		Logger:   a.Logger,
		Addr:     a.AuxListenOn,
		Gatherer: a.PrometheusRegistry,
		IsReady:  generic.IsReady,
		Debug:    a.Debug,
	}

	// Events
	eventsScheme := runtime.NewScheme()
	// we use ConfigMapLock which emits events for ConfigMap and hence we need (only) core_v1 types for it
	if err = core_v1.AddToScheme(eventsScheme); err != nil {
		return err
	}

	// Start events recorder
	eventBroadcaster := record.NewBroadcaster()
	loggingWatch := eventBroadcaster.StartLogging(a.Logger.Sugar().Infof)
	defer loggingWatch.Stop()
	recordingWatch := eventBroadcaster.StartRecordingToSink(&core_v1client.EventSinkImpl{Interface: a.MainClient.CoreV1().Events(meta_v1.NamespaceNone)})
	defer recordingWatch.Stop()
	recorder := eventBroadcaster.NewRecorder(eventsScheme, core_v1.EventSource{Component: a.Name})

	var auxErr error
	defer func() {
		if auxErr != nil && (retErr == context.DeadlineExceeded || retErr == context.Canceled) {
			retErr = auxErr
		}
	}()

	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stage.StartWithContext(func(metricsCtx context.Context) {
		defer cancel() // if auxSrv fails to start it signals the whole program that it should shut down
		defer logz.LogStructuredPanic()
		auxErr = auxSrv.Run(metricsCtx)
	})

	// Leader election
	if a.LeaderElectionOptions.LeaderElect {
		a.Logger.Info("Starting leader election", logz.NamespaceName(a.LeaderElectionOptions.ConfigMapNamespace))
		ctx, err = options.DoLeaderElection(ctx, a.Logger, a.Name, a.LeaderElectionOptions, a.MainClient.CoreV1(), recorder)
		if err != nil {
			return err
		}
	}
	return generic.Run(ctx)
}

// CancelOnInterrupt calls f when os.Interrupt or SIGTERM is received.
// It ignores subsequent interrupts on purpose - program should exit correctly after the first signal.
func CancelOnInterrupt(ctx context.Context, f context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-c:
			f()
		}
	}()
}

func NewFromFlags(name string, controllers []ctrl.Constructor, flagset *flag.FlagSet, arguments []string) (*App, error) {
	a := App{
		Name:        name,
		Controllers: controllers,
	}
	for _, cntrlr := range controllers {
		cntrlr.AddFlags(flagset)
	}

	flagset.BoolVar(&a.Debug, "debug", false, "Enables pprof and prefetcher dump endpoints")
	flagset.StringVar(&a.AuxListenOn, "aux-listen-on", defaultAuxServerAddr, "Auxiliary address to listen on. Used for Prometheus metrics server and pprof endpoint. Empty to disable")

	options.BindLeaderElectionFlags(name, &a.LeaderElectionOptions, flagset)
	options.BindGenericNamespacedControllerFlags(&a.GenericNamespacedControllerOptions, flagset)
	options.BindRestClientFlags(&a.RestClientOptions, flagset)
	options.BindLoggerFlags(&a.LoggerOptions, flagset)

	if err := flagutil.ValidateFlags(flagset, arguments); err != nil {
		return nil, err
	}

	if err := flagset.Parse(arguments); err != nil {
		return nil, err
	}
	if errs := a.GenericNamespacedControllerOptions.DefaultAndValidate(); len(errs) > 0 {
		return nil, errors.NewAggregate(errs)
	}
	if errs := a.RestClientOptions.DefaultAndValidate(); len(errs) > 0 {
		return nil, errors.NewAggregate(errs)
	}

	var err error
	a.RestConfig, err = options.LoadRestClientConfig(name, a.RestClientOptions)
	if err != nil {
		return nil, err
	}

	a.Logger = options.LoggerFromOptions(a.LoggerOptions)

	// Clients
	a.MainClient, err = kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return nil, err
	}

	// Metrics
	a.PrometheusRegistry = prometheus.NewPedanticRegistry()
	err = a.PrometheusRegistry.Register(prometheus.NewProcessCollector(os.Getpid(), ""))
	if err != nil {
		return nil, err
	}
	err = a.PrometheusRegistry.Register(prometheus.NewGoCollector())
	if err != nil {
		return nil, err
	}

	return &a, nil
}
