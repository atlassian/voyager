package app

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	comp_v1_client "github.com/atlassian/voyager/pkg/composition/client"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"github.com/atlassian/voyager/pkg/releases"
	"github.com/atlassian/voyager/pkg/releases/deployinator/client"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/synchronization"
	"github.com/atlassian/voyager/pkg/util"
	prom_util "github.com/atlassian/voyager/pkg/util/prometheus"
	"github.com/go-openapi/strfmt"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	rbac_v1inf "k8s.io/client-go/informers/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

type ControllerConstructor struct {
	FlagConfigFile string
}

func (cc *ControllerConstructor) AddFlags(flagset ctrl.FlagSet) {
	flagset.StringVar(&cc.FlagConfigFile, "config", "config.yaml", "Configuration file")
}

func (cc *ControllerConstructor) New(config *ctrl.Config, cctx *ctrl.Context) (*ctrl.Constructed, error) {
	if config.Namespace != meta_v1.NamespaceAll {
		return nil, errors.Errorf("synchronization should not be namespaced (was passed %q)", config.Namespace)
	}

	opts, err := readAndValidateOptions(cc.FlagConfigFile)
	if err != nil {
		return nil, err
	}

	compClient, err := comp_v1_client.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	nsInf, err := cctx.MainClusterInformer(config, k8s.NamespaceGVK, core_v1inf.NewNamespaceInformer)
	if err != nil {
		return nil, err
	}

	configMapInf, err := cctx.MainInformer(config, k8s.ConfigMapGVK, core_v1inf.NewConfigMapInformer)
	if err != nil {
		return nil, err
	}

	rbGVK := rbac_v1.SchemeGroupVersion.WithKind(k8s.RoleBindingKind)
	rbInf, err := cctx.MainInformer(config, rbGVK, rbac_v1inf.NewRoleBindingInformer)
	if err != nil {
		return nil, err
	}

	crbGVK := rbac_v1.SchemeGroupVersion.WithKind(k8s.ClusterRoleBindingKind)
	crbInf, err := cctx.MainClusterInformer(config, crbGVK, rbac_v1inf.NewClusterRoleBindingInformer)
	if err != nil {
		return nil, err
	}

	crGVK := rbac_v1.SchemeGroupVersion.WithKind(k8s.ClusterRoleKind)
	crInf, err := cctx.MainClusterInformer(config, crGVK, rbac_v1inf.NewClusterRoleInformer)
	if err != nil {
		return nil, err
	}

	// need a client for talking to SC
	scHTTPClient := util.HTTPClient()
	scClient := servicecentral.NewServiceCentralClient(config.Logger, scHTTPClient, opts.ASAPClientConfig, opts.Providers.ServiceCentralURL)

	scErrorCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "service_central_poll_error_total",
			Help:      "Number of times we have failed polling service central",
		},
	)

	accessUpdateErrorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.AppName,
			Name:      "access_update_error_total",
			Help:      "Number of times we have failed updating the access details for a service",
		},
		[]string{"service_name"},
	)

	if err := prom_util.RegisterAll(config.Registry, scErrorCounter, accessUpdateErrorCounter); err != nil {
		return nil, err
	}

	deployinatorHTTPClient := client.NewHTTPClientWithConfig(strfmt.NewFormats(),
		client.DefaultTransportConfig().
			WithHost(opts.Providers.DeployinatorURL.Host).
			WithSchemes([]string{opts.Providers.DeployinatorURL.Scheme}))

	err = nsInf.AddIndexers(cache.Indexers{
		synchronization.NamespaceByServiceLabelIndexName: synchronization.NsServiceLabelIndexFunc,
	})
	if err != nil {
		return nil, err
	}

	store := store.NewMultiBasic()
	specCheck := specchecker.New(store)
	roleBindingObjectUpdater := updater.RoleBindingUpdater(rbInf.GetIndexer(), specCheck, config.MainClient)
	configMapObjectUpdater := updater.ConfigMapUpdater(configMapInf.GetIndexer(), specCheck, config.MainClient)
	namespaceObjectUpdater := updater.NamespaceUpdater(nsInf.GetIndexer(), specCheck, config.MainClient)
	clusterRoleObjectUpdater := updater.ClusterRoleUpdater(crInf.GetIndexer(), specCheck, config.MainClient)
	clusterRoleBindingObjectUpdater := updater.ClusterRoleBindingUpdater(crbInf.GetIndexer(), specCheck, config.MainClient)

	syncCntrl := &synchronization.Controller{
		Logger:       config.Logger,
		ReadyForWork: cctx.ReadyForWork,

		MainClient: config.MainClient,
		CompClient: compClient,

		NamespaceInformer: nsInf,
		ConfigMapInformer: configMapInf,

		ServiceCentral:    servicecentral.NewStore(config.Logger, scClient),
		ReleaseManagement: releases.NewReleaseManagement(deployinatorHTTPClient, config.Logger),
		ClusterLocation:   opts.Location.ClusterLocation(),

		ConfigMapUpdater:          configMapObjectUpdater,
		RoleBindingUpdater:        roleBindingObjectUpdater,
		NamespaceUpdater:          namespaceObjectUpdater,
		ClusterRoleUpdater:        clusterRoleObjectUpdater,
		ClusterRoleBindingUpdater: clusterRoleBindingObjectUpdater,

		ServiceCentralPollErrorCounter: scErrorCounter,
		AccessUpdateErrorCounter:       accessUpdateErrorCounter,

		AllowMutateServices: opts.AllowMutateServices,

		ServiceCache: make(map[voyager.ServiceName]*creator_v1.Service),
	}

	return &ctrl.Constructed{
		Interface: syncCntrl,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: core_v1.SchemeGroupVersion.WithKind(k8s.NamespaceKind),
	}
}
