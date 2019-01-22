package app

import (
	"context"
	"flag"
	"math/rand"
	"time"

	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/pkg/creator/server"
	"github.com/atlassian/voyager/pkg/creator/server/options"
	"github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/atlassian/voyager/pkg/util/crash"
	genericserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/logs"
)

const (
	serviceName = "creator"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	crash.InstallAPIMachineryLoggers()

	logs.InitLogs()
	defer logs.FlushLogs()

	namespace, err := apiserver.GetInClusterNamespace(apiserver.DefaultNamespace)
	if err != nil {
		return err
	}
	opts := options.NewCreatorServerOptions(genericserveroptions.NewProcessInfo(serviceName, namespace))
	cmd := server.NewServerCommand(opts, ctx.Done())
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	return cmd.Execute()
}
