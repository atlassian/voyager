package app

import (
	"context"
	"flag"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/pkg/replication"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	fakeServiceName = "voyager/replicationadmission"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(func(ctx context.Context) error {
		server, err := NewServerFromFlags(flag.CommandLine, os.Args[1:])
		if err != nil {
			return err
		}

		return server.Run(ctx)
	})
}

func NewServerFromFlags(fs *flag.FlagSet, arguments []string) (*util.HTTPServer, error) {
	var (
		logOpts        options.LoggerOptions
		restClientOpts options.RestClientOptions
	)
	options.BindLoggerFlags(&logOpts, fs)
	options.BindRestClientFlags(&restClientOpts, fs)

	configFile := fs.String("config", "config.yaml", "Configuration file")

	err := fs.Parse(arguments)
	if err != nil {
		return nil, err
	}
	logger := options.LoggerFromOptions(logOpts)
	defer logz.Sync(logger)

	restConfig, err := options.LoadRestClientConfig("replicationadmission", restClientOpts)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	options, err := readAndValidateOptions(*configFile)
	if err != nil {
		return nil, err
	}

	admissionServer, err := util.NewHTTPServer(fakeServiceName, logger, options.ServerConfig)
	if err != nil {
		return nil, err
	}

	router := admissionServer.GetRouter()
	replicatedLocations := make(sets.ClusterLocation)
	for _, loc := range options.Locations.Replicated {
		replicatedLocations.Insert(loc.ClusterLocation())
	}
	ac := replication.AdmissionContext{
		CurrentLocation:     options.Locations.Current.ClusterLocation(),
		ReplicatedLocations: replicatedLocations,
		AuthzClient:         kubeClient.AuthorizationV1().SubjectAccessReviews(),
	}
	err = ac.SetupAdmissionWebhooks(router)
	if err != nil {
		return nil, err
	}
	router.Get("/healthz/ping", func(_ http.ResponseWriter, _ *http.Request) {})

	return admissionServer, nil
}
