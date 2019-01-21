package app

import (
	"flag"
	"math/rand"
	"time"

	"github.com/atlassian/voyager/pkg/creator/server"
	"github.com/atlassian/voyager/pkg/creator/server/options"
	"github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/atlassian/voyager/pkg/util/crash"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/klog"
)

const (
	serviceName = "creator"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	crash.InstallAPIMachineryLoggers()

	logs.InitLogs()
	defer logs.FlushLogs()

	namespace, err := apiserver.GetInClusterNamespace(apiserver.DefaultNamespace)
	if err != nil {
		klog.Fatal(err)
	}
	stopCh := genericapiserver.SetupSignalHandler()
	opts := options.NewCreatorServerOptions(genericserveroptions.NewProcessInfo(serviceName, namespace))
	cmd := server.NewServerCommand(opts, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		klog.Fatal(err)
	}
}
