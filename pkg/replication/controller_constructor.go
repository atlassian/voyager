package replication

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/composition/client"
	compInf "github.com/atlassian/voyager/pkg/composition/informer"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

type ControllerConstructor struct {
	RemoteConfigFile string
	ConfigContext    string
	TLSCert          string
	TLSKey           string
}

func (cc *ControllerConstructor) AddFlags(flagset ctrl.FlagSet) {
	flagset.StringVar(&cc.RemoteConfigFile, "remote-config-file", "/config/remote-config.yaml", "Path of the file containing the remote cluster config.")
	flagset.StringVar(&cc.ConfigContext, "remote-config-context", "replication-remote", "Context of remote cluster in configuration file.")
	flagset.StringVar(&cc.TLSCert, "remote-tls-cert-file", "/etc/certs/tls.crt", "Path of the TLS certificate.")
	flagset.StringVar(&cc.TLSKey, "remote-tls-cert-key", "/etc/certs/tls.key", "Path of the TLS key.")
}

func (cc *ControllerConstructor) New(config *ctrl.Config, cctx *ctrl.Context) (*ctrl.Constructed, error) {
	if config.Namespace != meta_v1.NamespaceAll {
		return nil, errors.Errorf("replication should not be namespaced (was passed %q)", config.Namespace)
	}

	// Remote Client Config
	configAPI, err := clientcmd.LoadFromFile(cc.RemoteConfigFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load REST client configuration from file %q", cc.RemoteConfigFile)
	}
	remoteRestConfig, err := clientcmd.NewDefaultClientConfig(*configAPI, &clientcmd.ConfigOverrides{
		CurrentContext: cc.ConfigContext,
	}).ClientConfig()
	if err != nil {
		return nil, err
	}

	// Clients
	localClient, err := client.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	remoteClient, err := client.NewForConfig(remoteRestConfig)
	if err != nil {
		return nil, err
	}

	// Informers
	// Only a single informer can be registered with the ctrl library for a given GVK
	// We need to have two informers: local and remote
	// We register the remote one (rather than the local one) as the ctrl lib sanity
	// checks that items it pulls off the work queue exist in the informer cache
	// Because the local informer is not registered, we need to manually add event handlers
	// and then start it in the Run func

	// Create and register remote informer
	remoteInf := compInf.ServiceDescriptorInformer(remoteClient, config.ResyncPeriod)
	err = cctx.RegisterInformer(comp_v1.ServiceDescriptorGVK, remoteInf)
	if err != nil {
		return nil, err
	}

	// Create and manually register local informer
	localInf := compInf.ServiceDescriptorInformer(localClient, config.ResyncPeriod)
	localInf.AddEventHandler(&handlers.GenericHandler{
		Logger:    config.Logger,
		WorkQueue: cctx.WorkQueue,
		Gvk:       comp_v1.ServiceDescriptorGVK,
	})

	// Controller
	replCtrl := &Controller{
		logger:       config.Logger,
		readyForWork: cctx.ReadyForWork,

		localInformer:  localInf,
		remoteInformer: remoteInf,
		localClient:    localClient,
	}

	return &ctrl.Constructed{
		Interface: replCtrl,
	}, nil
}

func (cc *ControllerConstructor) Describe() ctrl.Descriptor {
	return ctrl.Descriptor{
		Gvk: comp_v1.SchemeGroupVersion.WithKind(comp_v1.ServiceDescriptorResourceKind),
	}
}
