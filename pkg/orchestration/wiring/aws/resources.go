package aws

import (
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/servicecatalog"
)

const (
	DynamoDB      voyager.ResourceType           = "DynamoDB"
	DynamoDBName  oap.ResourceType               = "dynamo-db"
	DynamoDBClass servicecatalog.ClassExternalID = "0dae543c-216b-4a08-87bd-aea7522c0cfd"
	DynamoDBPlan  servicecatalog.PlanExternalID  = "9b59fb3e-56eb-487d-863e-bf831ca4fa3f"
	DynamoPrefix  oap.EnvVarPrefix               = "DYNAMO"

	S3       voyager.ResourceType           = "S3"
	S3Name   oap.ResourceType               = "s3"
	S3Class  servicecatalog.ClassExternalID = "a6bf1e70-9bbb-4826-9793-75871cb540f1"
	S3Plan   servicecatalog.PlanExternalID  = "d8eca56a-9634-4e6f-a7c8-47e3bc76bc83"
	S3Prefix oap.EnvVarPrefix               = "S3"

	Cfn       voyager.ResourceType           = "CloudFormation"
	CfnName   oap.ResourceType               = "cloudformation"
	CfnClass  servicecatalog.ClassExternalID = "312ebba6-e3df-443f-a151-669a04f0619b"
	CfnPlan   servicecatalog.PlanExternalID  = "8933f0a5-b232-4319-9861-baaccece62fd"
	CfnPrefix oap.EnvVarPrefix               = "CF"
)

// All osb-aws-provider resources are 'almost' the same, differing only in the service/plan names,
// what they need passed in the ServiceEnvironment.
var ResourceTypes = map[voyager.ResourceType]wiringplugin.WiringPlugin{
	DynamoDB: Resource(DynamoDB, DynamoDBName, DynamoDBClass, DynamoDBPlan, dynamoDbServiceEnvironment, DynamoPrefix, true),
	S3:       Resource(S3, S3Name, S3Class, S3Plan, s3ServiceEnvironment, S3Prefix, false),
	Cfn:      Resource(Cfn, CfnName, CfnClass, CfnPlan, CfnServiceEnvironment, CfnPrefix, false),
}

func dynamoDbServiceEnvironment(env *oap.ServiceEnvironment) *oap.ServiceEnvironment {
	return &oap.ServiceEnvironment{
		NotificationEmail:            env.NotificationEmail,
		LowPriorityPagerdutyEndpoint: env.LowPriorityPagerdutyEndpoint,
		PagerdutyEndpoint:            env.PagerdutyEndpoint,
		Tags:                         env.Tags,
		PrimaryVpcEnvironment: &oap.VPCEnvironment{
			Region:     env.PrimaryVpcEnvironment.Region,
			EMRSubnet:  env.PrimaryVpcEnvironment.EMRSubnet,
			AppSubnets: env.PrimaryVpcEnvironment.AppSubnets,
		},
	}
}

func s3ServiceEnvironment(_ *oap.ServiceEnvironment) *oap.ServiceEnvironment {
	fallback := false
	return &oap.ServiceEnvironment{
		Fallback: &fallback,
	}
}

// See https://stash.atlassian.com/projects/MDATA/repos/viceroy/browse/src/main/resources/schemas/cloudformation.json#28
func CfnServiceEnvironment(env *oap.ServiceEnvironment) *oap.ServiceEnvironment {
	return &oap.ServiceEnvironment{
		LowPriorityPagerdutyEndpoint: env.LowPriorityPagerdutyEndpoint,
		PagerdutyEndpoint:            env.PagerdutyEndpoint,
		Tags:                         env.Tags,
		ServiceSecurityGroup:         env.ServiceSecurityGroup,
		NotificationEmail:            env.NotificationEmail,
		PrimaryVpcEnvironment: &oap.VPCEnvironment{
			AppSubnets:            env.PrimaryVpcEnvironment.AppSubnets,
			VPCID:                 env.PrimaryVpcEnvironment.VPCID,
			JumpboxSecurityGroup:  env.PrimaryVpcEnvironment.JumpboxSecurityGroup,
			InstanceSecurityGroup: env.PrimaryVpcEnvironment.InstanceSecurityGroup,
			SSLCertificateID:      env.PrimaryVpcEnvironment.SSLCertificateID,
			PrivateDNSZone:        env.PrimaryVpcEnvironment.PrivateDNSZone,
			PrivatePaasDNSZone:    env.PrimaryVpcEnvironment.PrivatePaasDNSZone,
			Label:                 env.PrimaryVpcEnvironment.Label,
			Region:                env.PrimaryVpcEnvironment.Region,
			Zones:                 env.PrimaryVpcEnvironment.Zones,
		},
	}
}
