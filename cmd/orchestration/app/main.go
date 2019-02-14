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
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/util/crash"
	"k8s.io/klog"
)

const (
	serviceName = "orchestration"
)

func Main() {
	CustomMain(registry.KnownWiringPlugins)
}

func CustomMain(plugins map[voyager.ResourceType]wiringplugin.WiringPlugin) {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(func(ctx context.Context) error {
		crash.InstallAPIMachineryLoggers()
		controllers := []ctrl.Constructor{
			&ControllerConstructor{
				Plugins: plugins,
				Tags:    ExampleTags,
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

func ExampleTags(
	_ voyager.ClusterLocation,
	_ wiringplugin.ClusterConfig,
	_ voyager.Location,
	_ voyager.ServiceName,
	_ orch_meta.ServiceProperties,
) map[voyager.Tag]string {
	return make(map[voyager.Tag]string)
}
