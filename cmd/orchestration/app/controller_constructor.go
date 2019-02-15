package app

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient "github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/orchestration"
	orchClient "github.com/atlassian/voyager/pkg/orchestration/client"
	orchInf "github.com/atlassian/voyager/pkg/orchestration/informer"
	orchUpdater "github.com/atlassian/voyager/pkg/orchestration/updater"
	"github.com/atlassian/voyager/pkg/orchestration/wiring"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	prom_util "github.com/atlassian/voyager/pkg/util/prometheus"
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
	Plugins        map[voyager.ResourceType]wiringplugin.WiringPlugin
	Tags           wiring.TagGenerator
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

	orchClientset, err := orchClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	bundleClient, err := smithClientset.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	// Create informers and entangler
	stateGVK := cc.Describe().Gvk
	stateInf := orchInf.StateInformer(orchClientset, config.Namespace, config.ResyncPeriod)
	err = cctx.RegisterInformer(stateGVK, stateInf)
	if err != nil {
		return nil, err
	}

	// while this is a cluster-wide informer, we filter states by grabbing states
	// by-namespace-index, and because the state informer above is already namespaced,
	// this informer doesn't need any additional work.
	nsInf, err := cctx.MainClusterInformer(config, k8s.NamespaceGVK, core_v1inf.NewNamespaceInformer)
	if err != nil {
		return nil, err
	}

	configMapInf, err := cctx.MainInformer(config, k8s.ConfigMapGVK, core_v1inf.NewConfigMapInformer)
	if err != nil {
		return nil, err
	}

	bundleGVK := smith_v1.SchemeGroupVersion.WithKind(smith_v1.BundleResourceKind)
	bundleInf := smithClient.BundleInformer(bundleClient, config.Namespace, config.ResyncPeriod)
	err = cctx.RegisterInformer(bundleGVK, bundleInf)
	if err != nil {
		return nil, err
	}

	entangler := &wiring.Entangler{
		Plugins:         cc.Plugins,
		ClusterLocation: opts.Location.ClusterLocation(),
		ClusterConfig:   toClusterConfig(opts.Cluster),
		Tags:            cc.Tags,
	}

	// Spec check
	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)

	// Object updater
	bundleObjectUpdater := orchUpdater.BundleUpdater(bundleInf.GetIndexer(), specCheck, bundleClient)

	// Counts the number of times our Processing attempts to mutate a State to
	// modify its conditions.
	stateTransitionsCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "state_transitions_total",
			Help:      "Records the number of times a State transitions into a new condition",
		},
		[]string{"namespace", "name", "type", "reason"},
	)

	if err = prom_util.RegisterAll(config.Registry, stateTransitionsCounter); err != nil {
		return nil, err
	}

	c := &orchestration.Controller{
		Logger:       config.Logger,
		Clock:        clock.RealClock{},
		ReadyForWork: cctx.ReadyForWork,

		StateInformer:     stateInf,
		BundleInformer:    bundleInf,
		NamespaceInformer: nsInf,
		ConfigMapInformer: configMapInf,
		StateClient:       orchClientset.OrchestrationV1(),
		Entangler:         entangler,

		BundleObjectUpdater: bundleObjectUpdater,

		StateTransitionsCounter: stateTransitionsCounter,
	}

	// The stateInformer allows other event handlers to retrieve states by
	// hitting the namespace name or by a ConfigMap namespace+name.
	err = stateInf.AddIndexers(cache.Indexers{
		cache.NamespaceIndex:                   cache.MetaNamespaceIndexFunc,
		orchestration.ByConfigMapNameIndexName: orchestration.ByConfigMapNameIndex,
	})
	if err != nil {
		return nil, err
	}

	bundleInf.AddEventHandler(&handlers.ControlledResourceHandler{
		Logger:          config.Logger,
		WorkQueue:       cctx.WorkQueue,
		ControllerIndex: nil, // TODO
		ControllerGvk:   stateGVK,
		Gvk:             smith_v1.BundleGVK,
	})

	nsInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    config.Logger,
		WorkQueue: cctx.WorkQueue,
		Lookup: func(o runtime.Object) ([]runtime.Object, error) {
			namespace := o.(*core_v1.Namespace)
			stateObjects, err := stateInf.GetIndexer().ByIndex(cache.NamespaceIndex, namespace.Name)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			objs := make([]runtime.Object, 0, len(stateObjects))

			for _, obj := range stateObjects {
				objs = append(objs, obj.(runtime.Object))
			}

			return objs, nil
		},
		Gvk: core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind),
	})

	configMapInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    config.Logger,
		WorkQueue: cctx.WorkQueue,
		Lookup: func(o runtime.Object) ([]runtime.Object, error) {
			configMap := o.(*core_v1.ConfigMap)
			namespace := configMap.GetNamespace()
			configMapName := configMap.GetName()

			stateObjects, err := stateInf.GetIndexer().ByIndex(orchestration.ByConfigMapNameIndexName,
				orchestration.ByConfigMapNameIndexKey(namespace, configMapName))
			if err != nil {
				return nil, errors.WithStack(err)
			}
			objs := make([]runtime.Object, 0, len(stateObjects))

			for _, obj := range stateObjects {
				objs = append(objs, obj.(runtime.Object))
			}

			return objs, nil
		},
		Gvk: core_v1.SchemeGroupVersion.WithKind(k8s.ConfigMapKind),
	})

	return &ctrl.Constructed{
		Interface: c,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind),
	}
}

func toClusterConfig(cluster options.Cluster) wiringplugin.ClusterConfig {
	return wiringplugin.ClusterConfig{
		ClusterDomainName: cluster.ClusterDomainName,
		KittClusterEnv:    cluster.KITTClusterEnv,
		Kube2iamAccount:   cluster.Kube2iamAccount,
	}
}
