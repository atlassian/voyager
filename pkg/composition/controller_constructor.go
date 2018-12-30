package composition

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	"github.com/atlassian/voyager/pkg/composition/client"
	compInf "github.com/atlassian/voyager/pkg/composition/informer"
	compUpdater "github.com/atlassian/voyager/pkg/composition/updater"
	formClient "github.com/atlassian/voyager/pkg/formation/client"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	prom_util "github.com/atlassian/voyager/pkg/util/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type ControllerConstructor struct {
	FlagConfigFile string
}

func (cc *ControllerConstructor) AddFlags(flagset ctrl.FlagSet) {
	flagset.StringVar(&cc.FlagConfigFile, "config", "config.yaml", "Configuration file")
}

func (cc *ControllerConstructor) New(config *ctrl.Config, cctx *ctrl.Context) (*ctrl.Constructed, error) {
	// Parse config
	opts, err := readAndValidateOptions(cc.FlagConfigFile)
	if err != nil {
		return nil, err
	}

	// Clients
	sdClient, err := client.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	formationClient, err := formClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	// Cluster-wide Informers
	// Because we sometimes take a namespace flag, some special handling needs
	// to be done for ServiceDescriptor and Namespace to ensure we only grab
	// the ones that manage the single namespace we want to manage.
	sdInf := compInf.ServiceDescriptorInformer(sdClient, config.ResyncPeriod)
	err = cctx.RegisterInformer(comp_v1.ServiceDescriptorGVK, sdInf)
	if err != nil {
		return nil, err
	}

	// The namespace informer pulls out ServiceDescriptors owned by the namespace
	// and the above namespace filtering should handle it.
	nsInf, err := cctx.MainClusterInformer(config, k8s.NamespaceGVK, core_v1inf.NewNamespaceInformer)
	if err != nil {
		return nil, err
	}

	// Namespace-scoped Informers
	ldGVK := form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind)
	ldInf, err := formInf.FormationInformer(config, cctx, formationClient, ldGVK, formInf.LocationDescriptorInformer)
	if err != nil {
		return nil, err
	}

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	// Object Updater
	ldUpdater := compUpdater.LocationDescriptorUpdater(ldInf.GetIndexer(), specCheck, formationClient)
	namespaceUpdater := updater.NamespaceUpdater(nsInf.GetIndexer(), specCheck, config.MainClient)

	serviceDescriptorTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "service_descriptor_transitions_total",
			Help:      "Records the number of times a ServiceDescriptor transitions into a new condition",
		},
		[]string{"name", "type", "reason"},
	)
	if err = prom_util.RegisterAll(config.Registry, serviceDescriptorTransitionsCounter); err != nil {
		return nil, err
	}

	// Controller
	compCntrl := &Controller{
		logger:       config.Logger,
		clock:        clock.RealClock{},
		readyForWork: cctx.ReadyForWork,
		namespace:    config.Namespace,

		formationClient:   formationClient,
		compositionClient: sdClient,
		sdTransformer:     NewServiceDescriptorTransformer(opts.Location.ClusterLocation()),
		location:          opts.Location,

		nsUpdater: namespaceUpdater,
		ldUpdater: ldUpdater,
		ldIndexer: ldInf.GetIndexer(),
		nsIndexer: nsInf.GetIndexer(),

		serviceDescriptorTransitionsCounter: serviceDescriptorTransitionsCounter,
	}

	nsInf.AddEventHandler(&handlers.ControlledResourceHandler{
		Logger:          config.Logger,
		WorkQueue:       cctx.WorkQueue,
		ControllerIndex: nil,
		ControllerGvk:   comp_v1.ServiceDescriptorGVK,
		Gvk:             core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind),
	})

	lookupNamespaceOwner := func(obj runtime.Object) ([]runtime.Object, error) {
		ns, exists, err := nsInf.GetIndexer().GetByKey(obj.(meta_v1.Object).GetNamespace())
		if err != nil {
			return nil, err
		}

		if !exists {
			// Namespace is possibly deleted, which means theres no owner for the namespace
			// which means there's probably nothing to queue up.
			return []runtime.Object{}, nil
		}

		ref := meta_v1.GetControllerOf(ns.(meta_v1.Object))
		if ref != nil && ref.APIVersion == comp_v1.SchemeGroupVersion.String() && ref.Kind == comp_v1.ServiceDescriptorResourceKind {
			sd, sdExists, sdErr := sdInf.GetIndexer().GetByKey(ref.Name)
			if sdErr != nil {
				return nil, sdErr
			}
			if !sdExists {
				// SD is missing, which means something *just* deleted it... the namespace
				// should be gone now too because of the ownerreference
				return []runtime.Object{}, nil
			}
			return []runtime.Object{sd.(*comp_v1.ServiceDescriptor)}, nil
		}

		return []runtime.Object{}, nil
	}

	ldInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    config.Logger,
		WorkQueue: cctx.WorkQueue,
		Lookup:    lookupNamespaceOwner,
		Gvk:       form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind),
	})

	err = ldInf.AddIndexers(cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	if err != nil {
		return nil, err
	}

	err = nsInf.AddIndexers(cache.Indexers{
		nsServiceNameIndex: nsServiceNameIndexFunc,
	})
	if err != nil {
		return nil, err
	}

	return &ctrl.Constructed{
		Interface: compCntrl,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind),
	}
}
