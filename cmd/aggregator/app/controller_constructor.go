package app

import (
	"context"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/aggregator"
	agg_v1 "github.com/atlassian/voyager/pkg/apis/aggregator/v1"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	clientset "k8s.io/cluster-registry/pkg/client/clientset/versioned"
	informers "k8s.io/cluster-registry/pkg/client/informers/externalversions"
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
	ConfigFile string
}

func (cc *ControllerConstructor) AddFlags(fs ctrl.FlagSet) {
	fs.StringVar(&cc.ConfigFile, "config", "config.yaml", "config file")

	// TODO nislamov: Copy of ugly temporary hack to bypass flag validation
	additionalFlags := []string{
		"tls-cert-file",
		"tls-private-key-file",
		"secure-port",
		"kubeconfig",
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
		return nil, errors.Errorf("aggregator should not be namespaced (was passed %q)", config.Namespace)
	}

	opts, err := readAndValidateOptions(cc.ConfigFile)
	if err != nil {
		return nil, err
	}

	clusterInformer, err := createClusterInformer(config, cctx)

	if err != nil {
		return nil, err
	}

	r, err := util.NewRouter(config.AppName, config.Logger)
	if err != nil {
		return nil, err
	}

	r.Use(cctx.Middleware)

	_, err = aggregator.NewAPI(config.Logger, r, clusterInformer, opts.ASAPConfig, voyager.Location{
		EnvType: opts.Location.EnvType,
		Account: opts.Location.Account,
		Region:  opts.Location.Region,
	}, opts.APILocation, config.Registry, opts.EnvironmentWhitelist)
	if err != nil {
		return nil, err
	}

	apiserverStarter := &APIServerRunner{
		AggregatorHandler: r,
	}

	return &ctrl.Constructed{
		Server: NewReadyAPIServer(apiserverStarter, cctx.ReadyForWork),
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: agg_v1.AggregateGvk,
	}
}

func createClusterInformer(config *ctrl.Config, cctx *ctrl.Context) (cache.SharedIndexInformer, error) {
	clusterGVK := cr_v1a1.SchemeGroupVersion.WithKind("clusters")
	clusterClient, err := clientset.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	clusterInformer := informers.NewSharedInformerFactory(clusterClient, config.ResyncPeriod).Clusterregistry().V1alpha1().Clusters().Informer()

	err = clusterInformer.AddIndexers(cache.Indexers{
		aggregator.ByClusterLabelIndexName: aggregator.ByClusterLabelIndex,
	})

	if err != nil {
		return nil, err
	}

	err = cctx.RegisterInformer(clusterGVK, clusterInformer)
	return clusterInformer, err
}
