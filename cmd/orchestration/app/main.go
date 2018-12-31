package app

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
	"github.com/atlassian/voyager/pkg/orchestration/wiring/legacy"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/util/crash"
)

const (
	serviceName = "orchestration"
)

func Main() {
	CustomMain(emptyLegacyConfigFunc, registry.KnownWiringPlugins)
}

func CustomMain(getLegacyConfigFunc func(location voyager.Location) *legacy.Config, plugins map[voyager.ResourceType]wiringplugin.WiringPlugin) {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(func(ctx context.Context) error {
		crash.InstallAPIMachineryLoggers()
		controllers := []ctrl.Constructor{
			&ControllerConstructor{
				GetLegacyConfigFunc: getLegacyConfigFunc,
				Plugins:             plugins,
			},
		}

		// Set up controller
		a, err := ctrlApp.NewFromFlags(serviceName, controllers, flag.CommandLine, os.Args[1:])
		if err != nil {
			return err
		}

		return a.Run(ctx)
	})
}

func emptyLegacyConfigFunc(location voyager.Location) *legacy.Config {
	return &legacy.Config{}
}
