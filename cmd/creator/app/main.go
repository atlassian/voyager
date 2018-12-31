package app

import (
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/voyager/pkg/creator/server"
	"github.com/atlassian/voyager/pkg/creator/server/options"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/golang/glog"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/logs"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	crash.InstallAPIMachineryLoggers()

	logs.InitLogs()
	defer logs.FlushLogs()

	stopCh := genericapiserver.SetupSignalHandler()
	opts := options.NewCreatorServerOptions(os.Stdout, os.Stderr)
	cmd := server.NewServerCommand(opts, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		glog.Fatal(err)
	}
}
