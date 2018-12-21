package ctrl

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type FlagSet interface {
	DurationVar(p *time.Duration, name string, value time.Duration, usage string)
	IntVar(p *int, name string, value int, usage string)
	Float64Var(p *float64, name string, value float64, usage string)
	StringVar(p *string, name string, value string, usage string)
	BoolVar(p *bool, name string, value bool, usage string)
	UintVar(p *uint, name string, value uint, usage string)
	Int64Var(p *int64, name string, value int64, usage string)
	Uint64Var(p *uint64, name string, value uint64, usage string)
}

type Descriptor struct {
	// Group Version Kind of objects a controller can process.
	Gvk schema.GroupVersionKind
}

type Server interface {
	Run(context.Context) error
}

type Constructed struct {
	// Interface holds an optional controller interface.
	Interface Interface
	// Server holds an optional server interface.
	Server Server
}

type Constructor interface {
	AddFlags(FlagSet)
	// New constructs a new controller and/or server.
	// If it constructs a controller, it must register an informer for the GVK controller
	// handles via Context.RegisterInformer().
	New(*Config, *Context) (*Constructed, error)
	Describe() Descriptor
}

type Interface interface {
	Run(context.Context)
	Process(*ProcessContext) (retriable bool, err error)
}

type WorkQueueProducer interface {
	// Add adds an item to the workqueue.
	Add(QueueKey)
}

type ProcessContext struct {
	Logger *zap.Logger
	Object runtime.Object
}

type QueueKey struct {
	Namespace string
	Name      string
}

type Config struct {
	AppName      string
	Logger       *zap.Logger
	Namespace    string
	ResyncPeriod time.Duration
	Registry     prometheus.Registerer

	RestConfig *rest.Config
	MainClient kubernetes.Interface
}

type Operation string

const (
	UpdatedOperation Operation = "updated"
	DeletedOperation Operation = "deleted"
	AddedOperation   Operation = "added"
)

func (o Operation) String() string {
	return string(o)
}

type Context struct {
	// ReadyForWork is a function that the controller must call from its Run() method once it is ready to
	// process work using it's Process() method. This should be used to delay processing while some initialization
	// is being performed.
	ReadyForWork func()
	// Middleware is the standard middleware that is supposed to be used to wrap the http handler of the server.
	Middleware func(http.Handler) http.Handler
	// Will contain all informers once Generic controller constructs all controllers.
	// This is a read only field, must not be modified.
	Informers map[schema.GroupVersionKind]cache.SharedIndexInformer
	// Will contain all controllers once Generic controller constructs them.
	// This is a read only field, must not be modified.
	Controllers map[schema.GroupVersionKind]Interface
	WorkQueue   WorkQueueProducer
}

func (c *Context) RegisterInformer(gvk schema.GroupVersionKind, inf cache.SharedIndexInformer) error {
	if _, ok := c.Informers[gvk]; ok {
		return errors.New("informer with this GVK has been registered already")
	}
	if c.Informers == nil {
		c.Informers = make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	}
	c.Informers[gvk] = inf
	return nil
}

func (c *Context) MainInformer(config *Config, gvk schema.GroupVersionKind, f func(kubernetes.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.MainClient, config.Namespace, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func (c *Context) MainClusterInformer(config *Config, gvk schema.GroupVersionKind, f func(kubernetes.Interface, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.MainClient, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
