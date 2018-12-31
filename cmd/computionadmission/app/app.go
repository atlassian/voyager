package app

import (
	"context"
	"flag"
	"net/http"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/pkg/execution/computionadmission"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// NewServerFromFlags builds a new Server object based on Flags
func NewServerFromFlags(name string, fs *flag.FlagSet, arguments []string) (*Server, error) {
	logOpts := options.LoggerOptions{}
	options.BindLoggerFlags(&logOpts, fs)

	configFile := fs.String("config", "config.yaml", "Configuration file")

	err := fs.Parse(arguments)
	if err != nil {
		return nil, err
	}
	logger := options.LoggerFromOptions(logOpts)
	defer logz.Sync(logger)

	opts, err := readAndValidateOptions(*configFile)
	if err != nil {
		return nil, err
	}

	// create informers
	cfmInformer, nsInformer, err := buildInformers(name)
	if err != nil {
		return nil, err
	}

	admissionServer, err := util.NewHTTPServer(name, logger, opts.ServerConfig)
	if err != nil {
		return nil, err
	}

	router := admissionServer.GetRouter()
	admissionCtx := computionadmission.AdmissionContext{
		ConfigMapInformer:       cfmInformer,
		NamespaceInformer:       nsInformer,
		EnforcePRGB:             opts.EnforcePRGB,
		CompliantDockerPrefixes: opts.CompliantDockerPrefixes}
	err = admissionCtx.SetupAdmissionWebhooks(router)
	if err != nil {
		return nil, err
	}
	router.Get("/healthz/ping", func(_ http.ResponseWriter, _ *http.Request) {})

	s := Server{
		Name:       name,
		Informers:  []cache.SharedIndexInformer{cfmInformer, nsInformer},
		HTTPServer: admissionServer,
	}

	return &s, nil
}

// Server is wrapper all computionadmission servers
type Server struct {
	Name       string
	Informers  []cache.SharedIndexInformer
	HTTPServer *util.HTTPServer
}

// Run launches all the informers and then starts HTTPServer
func (s *Server) Run(ctx context.Context) error {
	stgr := stager.New()
	defer stgr.Shutdown()

	// launch infomers and wait their cache to be synced
	syncedFuncs := make([]cache.InformerSynced, 0, len(s.Informers))

	for _, infr := range s.Informers {
		stage := stgr.NextStage()
		stage.StartWithChannel(infr.Run)
		syncedFuncs = append(syncedFuncs, infr.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), syncedFuncs...) {
		return errors.New("failed to sync cache for all the informers for compution webhook")
	}

	return s.HTTPServer.Run(ctx)
}

func buildInformers(serviceName string) (cache.SharedIndexInformer, cache.SharedIndexInformer, error) {
	// use default in-cluster config and also default APIQPS (options.DefaultAPIQPS)
	restClientOpts := options.RestClientOptions{ClientConfigFileFrom: "in-cluster"}
	errs := restClientOpts.DefaultAndValidate()
	if len(errs) > 0 {
		return nil, nil, utilerrors.NewAggregate(errs)
	}
	restConfig, err := options.LoadRestClientConfig(serviceName, restClientOpts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create client rest config")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create k8s clientset")
	}

	configMapInformer := core_v1inf.NewConfigMapInformer(clientset, meta_v1.NamespaceAll, options.DefaultResyncPeriod, cache.Indexers{})
	namespaceInformer := core_v1inf.NewNamespaceInformer(clientset, options.DefaultResyncPeriod, cache.Indexers{})
	return configMapInformer, namespaceInformer, nil
}
