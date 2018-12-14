package main

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/ctrl"
	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/cmd/orchestration/app"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/util/crash"
)

const (
	serviceName = "orchestration"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	crash.InstallAPIMachineryLoggers()
	controllers := []ctrl.Constructor{
		&app.ControllerConstructor{
			GetLegacyConfigFunc: func(location *voyager.Location) *legacy.Config {
				return &legacy.Config{}
			},
			Plugins: registry.KnownWiringPlugins,
		},
	}

	// Set up controller
	a, err := ctrlApp.NewFromFlags(serviceName, controllers, flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}

	return a.Run(ctx)
}
