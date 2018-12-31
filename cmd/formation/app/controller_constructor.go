package app

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/formation"
	formClient "github.com/atlassian/voyager/pkg/formation/client"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	formUpdater "github.com/atlassian/voyager/pkg/formation/updater"
	"github.com/atlassian/voyager/pkg/k8s"
	orchClient "github.com/atlassian/voyager/pkg/orchestration/client"
	orchInf "github.com/atlassian/voyager/pkg/orchestration/informer"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
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
	// This should have been initialized by now
	opts, err := readAndValidateOptions(cc.FlagConfigFile)
	if err != nil {
		return nil, err
	}

	// Clients
	formationClient, err := formClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	orchestrationClient, err := orchClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	// Informers
	ldGVK := form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind)
	ldInf, err := formInf.FormationInformer(config, cctx, formationClient, ldGVK, formInf.LocationDescriptorInformer)
	if err != nil {
		return nil, err
	}

	stateGVK := orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind)
	stateInf, err := orchInf.OrchestrationInformer(config, cctx, orchestrationClient, stateGVK, orchInf.StateInformer)
	if err != nil {
		return nil, err
	}

	configMapInf, err := cctx.MainInformer(config, k8s.ConfigMapGVK, core_v1inf.NewConfigMapInformer)
	if err != nil {
		return nil, err
	}

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	// Object Updater
	stateObjectUpdater := formUpdater.StateUpdater(stateInf.GetIndexer(), specCheck, orchestrationClient)

	// Metrics
	ldTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "ld_transitions_total",
			Help:      "Records the number of times a LocationDescriptor transitions into a new condition",
		},
		[]string{"namespace", "name", "type", "reason"},
	)

	err = config.Registry.Register(ldTransitionsCounter)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Controller
	formCntrl := &formation.Controller{
		Logger:       config.Logger,
		Clock:        clock.RealClock{},
		ReadyForWork: cctx.ReadyForWork,

		LDInformer:        ldInf,
		StateInformer:     stateInf,
		LDClient:          formationClient.FormationV1(),
		ConfigMapInformer: configMapInf,

		LDTransitionsCounter: ldTransitionsCounter,

		Location: opts.Location,

		StateObjectUpdater: stateObjectUpdater,
	}

	stateInf.AddEventHandler(&handlers.ControlledResourceHandler{
		Logger:          config.Logger,
		WorkQueue:       cctx.WorkQueue,
		ControllerIndex: nil, // TODO
		ControllerGvk:   ldGVK,
		Gvk:             orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind),
	})

	// Add an indexer for Release ConfigMap -> LocationDescriptor(s) lookups
	err = ldInf.AddIndexers(cache.Indexers{
		cache.NamespaceIndex:                   cache.MetaNamespaceIndexFunc,
		formation.ByReleaseConfigNameIndexName: formation.ByReleaseConfigMapNameIndex,
	})
	if err != nil {
		return nil, err
	}

	configMapInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    config.Logger,
		WorkQueue: cctx.WorkQueue,
		Lookup: func(o runtime.Object) ([]runtime.Object, error) {
			configMap := o.(*core_v1.ConfigMap)
			namespace := configMap.GetNamespace()
			configMapName := configMap.GetName()

			ldObjects, err := ldInf.GetIndexer().ByIndex(formation.ByReleaseConfigNameIndexName,
				formation.ByConfigMapNameIndexKey(namespace, configMapName))
			if err != nil {
				return nil, errors.WithStack(err)
			}
			objs := make([]runtime.Object, 0, len(ldObjects))

			for _, obj := range ldObjects {
				objs = append(objs, obj.(runtime.Object))
			}

			return objs, nil
		},
	})

	return &ctrl.Constructed{
		Interface: formCntrl,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind),
	}
}
