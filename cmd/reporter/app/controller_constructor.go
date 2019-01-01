package app

import (
	"context"
	"time"

	"github.com/atlassian/ctrl"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	bundleClient "github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	"github.com/atlassian/voyager"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	form_v1 "github.com/atlassian/voyager/pkg/apis/formation/v1"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	compClient "github.com/atlassian/voyager/pkg/composition/client"
	compInf "github.com/atlassian/voyager/pkg/composition/informer"
	formClient "github.com/atlassian/voyager/pkg/formation/client"
	formInf "github.com/atlassian/voyager/pkg/formation/informer"
	"github.com/atlassian/voyager/pkg/k8s"
	opsClient "github.com/atlassian/voyager/pkg/ops/client"
	opsInformer "github.com/atlassian/voyager/pkg/ops/informer"
	orchClient "github.com/atlassian/voyager/pkg/orchestration/client"
	orchInf "github.com/atlassian/voyager/pkg/orchestration/informer"
	"github.com/atlassian/voyager/pkg/reporter"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/informers"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apps_v1inf "k8s.io/client-go/informers/apps/v1"
	autoscaling_v2b1Inf "k8s.io/client-go/informers/autoscaling/v2beta1"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	ext_v1beta1Inf "k8s.io/client-go/informers/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	GroupVersionKindNameComponents = 4
)

type ReadyServer struct {
	server       ctrl.Server
	readyForWork func()
}

func NewReadyAPIServer(server ctrl.Server, readyForWork func()) ctrl.Server {
	return &ReadyServer{
		server:       server,
		readyForWork: readyForWork,
	}
}

func (s *ReadyServer) Run(ctx context.Context) error {
	s.readyForWork()
	return s.server.Run(ctx)
}

type ControllerConstructor struct {
	ConfigFile            string
	ServiceCatalogSupport bool
}

func (cc *ControllerConstructor) AddFlags(fs ctrl.FlagSet) {
	fs.StringVar(&cc.ConfigFile, "config", "config.yaml", "config file")

	// TODO nislamov: Copy of ugly temporary hack to bypass flag validation
	additionalFlags := []string{
		"tls-cert-file",
		"tls-private-key-file",
		"secure-port",
		"kubeconfig",
		"authentication-kubeconfig",
		"authorization-kubeconfig",
		"client-ca-file",
		"local",
		"audit-policy-file",
		"audit-log-path",
		"audit-log-maxsize",
		"audit-log-maxbackup",
		"audit-log-maxage",
	}
	s := ""
	for _, f := range additionalFlags {
		fs.StringVar(&s, f, "", "")
	}
}

func (cc *ControllerConstructor) New(config *ctrl.Config, cctx *ctrl.Context) (*ctrl.Constructed, error) {
	if config.Namespace != meta_v1.NamespaceAll {
		return nil, errors.Errorf("reporter should not be namespaced (was passed %q)", config.Namespace)
	}

	opts, err := readAndValidateOptions(cc.ConfigFile)
	if err != nil {
		return nil, err
	}

	type informerInfo struct {
		newInformer func(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer
		gvk         schema.GroupVersionKind
	}
	for _, informerInfo := range []informerInfo{
		{core_v1inf.NewConfigMapInformer, core_v1.SchemeGroupVersion.WithKind(k8s.ConfigMapKind)},
		{core_v1inf.NewSecretInformer, core_v1.SchemeGroupVersion.WithKind(k8s.SecretKind)},
		{ext_v1beta1Inf.NewIngressInformer, ext_v1beta1.SchemeGroupVersion.WithKind(k8s.IngressKind)},
		{core_v1inf.NewEventInformer, core_v1.SchemeGroupVersion.WithKind(k8s.EventKind)},
		{apps_v1inf.NewDeploymentInformer, k8s.DeploymentGVK},
		{apps_v1inf.NewReplicaSetInformer, k8s.ReplicaSetGVK},
		{core_v1inf.NewPodInformer, k8s.PodGVK},
		{autoscaling_v2b1Inf.NewHorizontalPodAutoscalerInformer, k8s.HorizontalPodAutoscalerGVK},
	} {
		informer := informerInfo.newInformer(config.MainClient, meta_v1.NamespaceAll, config.ResyncPeriod, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
		if err := cctx.RegisterInformer(informerInfo.gvk, informer); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	err = cc.generateVoyagerInformers(config, cctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	nsInf, err := cctx.MainClusterInformer(config, k8s.NamespaceGVK, core_v1inf.NewNamespaceInformer)
	if err != nil {
		return nil, err
	}

	err = nsInf.AddIndexers(cache.Indexers{
		reporter.ByServiceNameLabelIndexName: reporter.ByServiceNameLabelIndex,
	})

	if err != nil {
		return nil, err
	}

	r, err := util.NewRouter(config.AppName, config.Logger)
	if err != nil {
		return nil, err
	}

	r.Use(cctx.Middleware)

	reportingAPI, err := reporter.NewReportingAPI(config.Logger, r, cctx.Informers, opts.ASAPConfig, voyager.Location{
		EnvType: opts.Location.EnvType,
		Account: opts.Location.Account,
		Region:  opts.Location.Region,
	}, opts.APILocation, config.Registry)
	if err != nil {
		return nil, err
	}

	// Setup listener for Route Objects
	opsClientset, err := opsClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	routeGVK := cc.Describe().Gvk
	routeInf, err := opsInformer.OpsInformer(config, cctx, opsClientset, routeGVK, opsInformer.RouteInformer)
	if err != nil {
		return nil, err
	}

	routeInf.AddEventHandler(reportingAPI)

	apiserverStarter := &APIServerRunner{
		ReportHandler: r,
	}

	return &ctrl.Constructed{
		Server: NewReadyAPIServer(apiserverStarter, cctx.ReadyForWork),
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: ops_v1.RouteGvk,
	}
}

func (cc *ControllerConstructor) generateVoyagerInformers(config *ctrl.Config, cctx *ctrl.Context) error {
	voyagerInformers := []cache.SharedIndexInformer{}
	composition, err := compClient.NewForConfig(config.RestConfig)
	if err != nil {
		return err
	}

	compGVK := comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind)
	compInformer, err := compInf.CompositionInformer(config, cctx, composition, compGVK, compInf.ServiceDescriptorInformer)
	if err != nil {
		return err
	}
	voyagerInformers = append(voyagerInformers, compInformer)

	formationClient, err := formClient.NewForConfig(config.RestConfig)
	if err != nil {
		return err
	}

	formGVK := form_v1.SchemeGroupVersion.WithKind(form_v1.LocationDescriptorResourceKind)
	formInformer, err := formInf.FormationInformer(config, cctx, formationClient, formGVK, formInf.LocationDescriptorInformer)
	if err != nil {
		return err
	}

	voyagerInformers = append(voyagerInformers, formInformer)

	orchestrationClient, err := orchClient.NewForConfig(config.RestConfig)
	if err != nil {
		return err
	}
	stateGVK := orch_v1.SchemeGroupVersion.WithKind(orch_v1.StateResourceKind)
	orchInformer, err := orchInf.OrchestrationInformer(config, cctx, orchestrationClient, stateGVK, orchInf.StateInformer)
	if err != nil {
		return err
	}

	voyagerInformers = append(voyagerInformers, orchInformer)

	smithClient, err := smithClientset.NewForConfig(config.RestConfig)
	if err != nil {
		return err
	}
	bundleGVK := smith_v1.SchemeGroupVersion.WithKind(smith_v1.BundleResourceKind)
	bundleInf, err := informers.SmithInformer(config, cctx, smithClient, bundleGVK, bundleClient.BundleInformer)

	if err != nil {
		return err
	}

	voyagerInformers = append(voyagerInformers, bundleInf)

	if cc.ServiceCatalogSupport {
		scClient, e := scClientset.NewForConfig(config.RestConfig)
		if e != nil {
			return e
		}

		bindGVK := sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding")
		instanceGVK := sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")

		bindingInf, e := informers.SvcCatInformer(config, cctx, scClient, bindGVK, sc_v1b1inf.NewServiceBindingInformer)
		if e != nil {
			return e
		}

		instanceInf, e := informers.SvcCatInformer(config, cctx, scClient, instanceGVK, sc_v1b1inf.NewServiceInstanceInformer)
		if e != nil {
			return e
		}

		voyagerInformers = append(voyagerInformers, instanceInf, bindingInf)
	}

	for _, inf := range voyagerInformers {
		err = inf.AddIndexers(cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
