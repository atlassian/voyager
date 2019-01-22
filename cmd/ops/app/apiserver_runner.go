package app

import (
	"context"
	"flag"
	"net/http"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager/pkg/ops/server"
	"github.com/atlassian/voyager/pkg/ops/server/options"
	"github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericserveroptions "k8s.io/apiserver/pkg/server/options"
)

var _ ctrl.Server = &APIServerRunner{}

type APIServerRunner struct {
	OpsHandler http.Handler
}

func (s *APIServerRunner) Run(context.Context) error {
	namespace, err := apiserver.GetInClusterNamespace(apiserver.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}

	stopCh := genericapiserver.SetupSignalHandler()
	opts := options.NewOpsServerOptions(s.OpsHandler, genericserveroptions.NewProcessInfo(serviceName, namespace))
	cmd := server.NewServerCommand(opts, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	return cmd.Execute()
}
