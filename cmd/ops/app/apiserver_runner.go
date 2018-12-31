package app

import (
	"context"
	"flag"
	"net/http"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager/pkg/ops/server"
	"github.com/atlassian/voyager/pkg/ops/server/options"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/logs"
)

var _ ctrl.Server = &APIServerRunner{}

type APIServerRunner struct {
	OpsHandler http.Handler
}

func (s *APIServerRunner) Run(context.Context) error {
	logs.InitLogs()
	defer logs.FlushLogs()

	stopCh := genericapiserver.SetupSignalHandler()
	opts := options.NewOpsServerOptions(s.OpsHandler)
	cmd := server.NewServerCommand(opts, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	return cmd.Execute()
}
