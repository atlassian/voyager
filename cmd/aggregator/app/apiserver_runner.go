package app

import (
	"context"
	"flag"
	"net/http"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager/pkg/aggregator/server"
	"github.com/atlassian/voyager/pkg/aggregator/server/options"
	"github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/logs"
)

var _ ctrl.Server = &APIServerRunner{}

type APIServerRunner struct {
	AggregatorHandler http.Handler
}

func (s *APIServerRunner) Run(context.Context) error {
	logs.InitLogs()
	defer logs.FlushLogs()
	namespace, err := apiserver.GetInClusterNamespace(apiserver.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}
	stopCh := genericapiserver.SetupSignalHandler()
	opts := options.NewAggregatorServerOptions(s.AggregatorHandler, genericserveroptions.NewProcessInfo(serviceName, namespace))
	cmd := server.NewServerCommand(opts, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	return cmd.Execute()
}
