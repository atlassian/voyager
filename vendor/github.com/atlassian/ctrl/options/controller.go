package options

import (
	"time"

	"github.com/atlassian/ctrl"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultResyncPeriod = 20 * time.Minute
	DefaultWorkers      = 2
)

type GenericControllerOptions struct {
	ResyncPeriod time.Duration
	Workers      uint
}

func (o *GenericControllerOptions) DefaultAndValidate() []error {
	var allErrors []error
	if o.ResyncPeriod == 0 {
		o.ResyncPeriod = DefaultResyncPeriod
	}
	if o.Workers == 0 {
		o.Workers = DefaultWorkers
	}
	return allErrors
}

func BindGenericControllerFlags(o *GenericControllerOptions, fs ctrl.FlagSet) {
	fs.DurationVar(&o.ResyncPeriod, "resync-period", DefaultResyncPeriod, "Resync period for informers")
	fs.UintVar(&o.Workers, "workers", DefaultWorkers, "Number of workers that handle events from informers")
}

type GenericNamespacedControllerOptions struct {
	GenericControllerOptions
	Namespace string
}

func BindGenericNamespacedControllerFlags(o *GenericNamespacedControllerOptions, fs ctrl.FlagSet) {
	BindGenericControllerFlags(&o.GenericControllerOptions, fs)
	fs.StringVar(&o.Namespace, "namespace", meta_v1.NamespaceAll, "Namespace to use. All namespaces are used if empty string or omitted")
}
