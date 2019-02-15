package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/atlassian/ctrl"
	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/cmd"
	orch_meta "github.com/atlassian/voyager/pkg/apis/orchestration/meta"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/registry"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util/crash"
	"k8s.io/klog"
)

const (
	serviceName = "orchestration"
)

func Main() {
	CustomMain(registry.KnownWiringPlugins(
		exampleDeveloperRole,
		exampleManagedPolicies,
		exampleVPC,
		exampleEnvironment,
	))
}

func CustomMain(plugins map[voyager.ResourceType]wiringplugin.WiringPlugin) {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(func(ctx context.Context) error {
		crash.InstallAPIMachineryLoggers()
		controllers := []ctrl.Constructor{
			&ControllerConstructor{
				Plugins: plugins,
				Tags:    exampleTags,
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

func exampleTags(
	_ voyager.ClusterLocation,
	_ wiringplugin.ClusterConfig,
	_ voyager.Location,
	_ voyager.ServiceName,
	_ orch_meta.ServiceProperties,
) map[voyager.Tag]string {
	return make(map[voyager.Tag]string)
}

func exampleDeveloperRole(_ voyager.Location) []string {
	return strings.Split(os.Getenv("PLUGIN_DEVELOPER_ROLE"), ",") //example
}
func exampleManagedPolicies(_ voyager.Location) []string {
	return strings.Split(os.Getenv("PLUGIN_MANAGED_POLICIES"), ",")
}
func exampleVPC(location voyager.Location) *oap.VPCEnvironment {
	return &oap.VPCEnvironment{
		VPCID:                 os.Getenv("PLUGIN_VPC_ID"),
		PrivateDNSZone:        os.Getenv("PLUGIN_PRIVATE_DNS_ZONE"),
		PrivatePaasDNSZone:    os.Getenv("PLUGIN_PRIVATE_PAAS_DNS_ZONE"),
		InstanceSecurityGroup: os.Getenv("PLUGIN_INSTANCE_SECURITY_GROUP"),
		JumpboxSecurityGroup:  os.Getenv("PLUGIN_JUMP_BOX_SECURITY_GROUP"),
		SSLCertificateID:      os.Getenv("PLUGIN_SSL_CERT_ID"),
		Label:                 location.Label,
		AppSubnets:            strings.Split(os.Getenv("PLUGIN_APP_SUBNETS"), ","),
		Zones:                 strings.Split(os.Getenv("PLUGIN_ZONES"), ","),
		Region:                location.Region,
		EMRSubnet:             os.Getenv("EMR_SUBNET"),
	}
}

func exampleEnvironment(_ voyager.Location) string {
	return os.Getenv("PLUGIN_ENVIRONMENT")
}
