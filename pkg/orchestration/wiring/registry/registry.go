package registry

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/aws"
	ec2compute_v2 "github.com/atlassian/voyager/pkg/orchestration/wiring/ec2compute/v2"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/kubeingress"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/postgres"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/rds"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/sqs"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/ups"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

var KnownWiringPlugins = map[voyager.ResourceType]wiringplugin.WiringPlugin{
	apik8scompute.ResourceType:  wiringplugin.StatusAdapter(k8scompute.WireUp),
	kubeingress.ResourceType:    wiringplugin.StatusAdapter(kubeingress.WireUp),
	ec2compute_v2.ResourceType:  wiringplugin.StatusAdapter(ec2compute_v2.WireUp),
	ups.ResourceType:            wiringplugin.StatusAdapter(ups.New().WireUp),
	aws.Cfn:                     aws.ResourceTypes[aws.Cfn],
	aws.DynamoDB:                aws.ResourceTypes[aws.DynamoDB],
	aws.S3:                      aws.ResourceTypes[aws.S3],
	postgres.ResourceType:       wiringplugin.StatusAdapter(postgres.New().WireUp),
	rds.ResourceType:            wiringplugin.StatusAdapter(rds.New().WireUp),
	sqs.ResourceType:            wiringplugin.StatusAdapter(sqs.WireUp),
	asapkey.ResourceType:        wiringplugin.StatusAdapter(asapkey.New().WireUp),
	apiinternaldns.ResourceType: wiringplugin.StatusAdapter(internaldns.New().WireUp),
}
