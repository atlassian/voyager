package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/ctrl"
	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/pkg/util/crash"
	"k8s.io/apiserver/pkg/util/logs"
)

const (
	serviceName = "reporter"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	crash.InstallAPIMachineryLoggers()
	logs.InitLogs()
	defer logs.FlushLogs()
	controllers := []ctrl.Constructor{
		&ControllerConstructor{
			ServiceCatalogSupport: true,
		},
	}

	a, err := ctrlApp.NewFromFlags(serviceName, controllers, flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	return a.Run(ctx)
}
