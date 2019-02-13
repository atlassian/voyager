package app

import (
	"context"
	"flag"

	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager"
	comp_v1_client "github.com/atlassian/voyager/pkg/composition/client"
	creator_v1_client "github.com/atlassian/voyager/pkg/creator/client"
	"github.com/atlassian/voyager/pkg/monitor"
	"github.com/atlassian/voyager/pkg/util/logz"
	sc_v1b1_client "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

type App struct {
	RestConfig        *rest.Config
	Logger            *zap.Logger
	Options           Options
	ServiceDescriptor string
}

func NewFromFlags(flagset *flag.FlagSet, arguments []string) (*App, error) {
	var (
		logOpts        options.LoggerOptions
		restClientOpts options.RestClientOptions
	)
	options.BindLoggerFlags(&logOpts, flagset)
	options.BindRestClientFlags(&restClientOpts, flagset)

	configFile := flagset.String("config", "config.yaml", "Configuration file")
	sd := flagset.String("service-descriptor", "{}", "JSON encoded ServiceDescriptor to use in test")

	err := flagset.Parse(arguments)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	opts, err := readAndValidateOptions(*configFile)
	if err != nil {
		return nil, err
	}

	restConfig, err := options.LoadRestClientConfig("monitor", restClientOpts)
	if err != nil {
		return nil, err
	}

	return &App{
		Logger:            options.LoggerFromOptions(logOpts),
		RestConfig:        restConfig,
		Options:           *opts,
		ServiceDescriptor: *sd,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer logz.Sync(a.Logger)

	composition, err := comp_v1_client.NewForConfig(a.RestConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	creator, err := creator_v1_client.NewForConfig(a.RestConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	sc, err := sc_v1b1_client.NewForConfig(a.RestConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	runID := uuid.New()
	m := &monitor.Monitor{
		Location: voyager.Location{
			Account: a.Options.Location.Account,
			EnvType: a.Options.Location.EnvType,
			Region:  a.Options.Location.Region,
		},
		Logger:                 a.Logger.With(zap.String("runID", runID)),
		ServiceDescriptorName:  a.Options.ServiceDescriptorName,
		ExpectedProcessingTime: a.Options.ExpectedProcessingTime,
		ServiceSpec:            a.Options.ServiceSpec,
		ServiceDescriptor:      a.ServiceDescriptor,

		ServiceDescriptorClient: composition.CompositionV1().ServiceDescriptors(),
		ServiceCatalogClient:    sc,
		CreatorServiceClient:    creator.CreatorV1().Services(),
	}
	return m.Run(ctx)
}
