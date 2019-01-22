package registry

import (
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/aws"
	ec2compute_v2 "github.com/atlassian/voyager/pkg/orchestration/wiring/ec2compute/v2"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/edge"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/kubeingress"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/postgres"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/rds"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/sqs"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/ups"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
)

type WireUpFunc func(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (r *wiringplugin.WiringResult, retriable bool, e error)

func (f WireUpFunc) WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (r *wiringplugin.WiringResult, retriable bool, e error) {
	return f(resource, context)
}

var KnownWiringPlugins = map[voyager.ResourceType]wiringplugin.WiringPlugin{
	apik8scompute.ResourceType: WireUpFunc(k8scompute.WireUp),
	kubeingress.ResourceType:   WireUpFunc(kubeingress.WireUp),
	ec2compute_v2.ResourceType: WireUpFunc(ec2compute_v2.WireUp),
	ups.ResourceType:           ups.New(),
	aws.Cfn:                    aws.ResourceTypes[aws.Cfn],
	aws.DynamoDB:               aws.ResourceTypes[aws.DynamoDB],
	aws.S3:                     aws.ResourceTypes[aws.S3],
	postgres.ResourceType:      postgres.New(),
	rds.ResourceType:           rds.New(),
	sqs.ResourceType:           WireUpFunc(sqs.WireUp),
	asapkey.ResourceType:       asapkey.New(),
	internaldns.ResourceType:   internaldns.New(),
	edge.ResourceType:          edge.New(),
}
