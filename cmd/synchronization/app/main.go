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
	"k8s.io/klog"
)

const (
	serviceName = "synchronization"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	crash.InstallAPIMachineryLoggers()
	controllers := []ctrl.Constructor{
		&ControllerConstructor{},
	}
	a, err := ctrlApp.NewFromFlags(serviceName, controllers, flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	return a.Run(ctx)
}
