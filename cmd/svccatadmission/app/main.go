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
	"github.com/atlassian/voyager/pkg/execution/svccatadmission"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/atlassian/voyager/pkg/util/logz"
)

const (
	fakeServiceName = "voyager/svccatadmission"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(func(ctx context.Context) error {
		crash.InstallAPIMachineryLoggers()
		server, err := NewServerFromFlags(flag.CommandLine, os.Args[1:])
		if err != nil {
			return err
		}
		return server.Run(ctx)
	})
}

func NewServerFromFlags(fs *flag.FlagSet, arguments []string) (*util.HTTPServer, error) {
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

	admissionServer, err := util.NewHTTPServer(fakeServiceName, logger, opts.ServerConfig)
	if err != nil {
		return nil, err
	}

	router := admissionServer.GetRouter()
	sca := &svccatadmission.SvcCatAdmission{
		ServiceCentralURL: opts.Providers.ServiceCentralURL,
		RPSURL:            opts.Providers.RPSURL,
		MicrosServerURL:   opts.Providers.MicrosServerURL,
		ASAPClientConfig:  opts.ASAPClientConfig,
		Logger:            logger,
		HTTPClient:        util.HTTPClient(),
	}
	sca.SetupAdmissionWebhooks(router)
	router.Get("/healthz/ping", func(_ http.ResponseWriter, _ *http.Request) {})

	return admissionServer, nil
}
