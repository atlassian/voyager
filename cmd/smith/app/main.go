package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/ctrl"
	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/cmd/smith/config"
	"k8s.io/klog"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	controllers := []ctrl.Constructor{
		&app.BundleControllerConstructor{
			Plugins: config.Plugins(),
		},
	}
	a, err := ctrlApp.NewFromFlags("smith", controllers, flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	return a.Run(ctx)
}
