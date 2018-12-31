package app

import (
	"github.com/atlassian/ctrl"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/ops"
	opsClient "github.com/atlassian/voyager/pkg/ops/client"
	opsInformer "github.com/atlassian/voyager/pkg/ops/informer"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/informers"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type ControllerConstructor struct {
	FlagConfigFile string
}

func (cc *ControllerConstructor) AddFlags(flagset ctrl.FlagSet) {
	flagset.StringVar(&cc.FlagConfigFile, "config", "config.yaml", "Configuration file")

	// TODO nislamov: Ugly temporary hack to bypass flag validation
	additionalFlags := []string{
		"tls-cert-file",
		"tls-private-key-file",
		"secure-port",
		"kubeconfig",
		"authentication-kubeconfig",
		"authorization-kubeconfig",
		"local",
		"audit-policy-file",
		"audit-log-path",
		"audit-log-maxsize",
		"audit-log-maxbackup",
		"audit-log-maxage",
	}
	s := ""
	for _, f := range additionalFlags {
		flagset.StringVar(&s, f, "", "")
	}
}

func (cc *ControllerConstructor) New(config *ctrl.Config, cctx *ctrl.Context) (*ctrl.Constructed, error) {
	// Ops cannot be namespace specific. Kill this flag.
	// ?: Does it make sense to have the Route CRD be namespaced?
	if config.Namespace != meta_v1.NamespaceAll {
		return nil, errors.Errorf("ops should not be namespaced (was passed %q)", config.Namespace)
	}

	// This should have been initialized by now
	opts, err := readAndValidateOptions(cc.FlagConfigFile)
	if err != nil {
		return nil, err
	}

	opsClientset, err := opsClient.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	scClient, err := scClientset.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	routeGVK := cc.Describe().Gvk
	routeInf := opsInformer.RouteInformer(opsClientset, config.Namespace, config.ResyncPeriod)
	err = cctx.RegisterInformer(routeGVK, routeInf)
	if err != nil {
		return nil, err
	}

	instanceGVK := sc_v1b1.SchemeGroupVersion.WithKind(k8s.ServiceInstanceKind)
	instanceInf, err := informers.SvcCatInformer(config, cctx, scClient, instanceGVK, sc_v1b1inf.NewServiceInstanceInformer)
	if err != nil {
		return nil, err
	}
	err = instanceInf.AddIndexers(cache.Indexers{
		ops.ServiceInstanceExternalIDIndex: ops.ServiceInstanceExternalIDIndexFunc,
	})
	if err != nil {
		return nil, err
	}

	apiRouter, err := util.NewRouter(config.AppName, config.Logger)
	if err != nil {
		return nil, err
	}
	apiRouter.Use(cctx.Middleware)

	opsAPI, err := ops.NewOpsAPI(config.Logger, opts.ASAPConfig, apiRouter, config.Registry, instanceInf)
	if err != nil {
		return nil, err
	}

	apiserverStarter := &APIServerRunner{
		OpsHandler: apiRouter,
	}

	return &ctrl.Constructed{
		Interface: &ops.Controller{
			RouteInformer: routeInf,
			ReadyForWork:  cctx.ReadyForWork,
			API:           opsAPI,
			Logger:        config.Logger,
		},
		Server: apiserverStarter,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: ops_v1.SchemeGroupVersion.WithKind(ops_v1.RouteResourceKind),
	}
}
