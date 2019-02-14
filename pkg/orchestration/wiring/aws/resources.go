package aws

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/servicecatalog"
	"github.com/pkg/errors"
)

const (
	DynamoDB      voyager.ResourceType           = "DynamoDB"
	DynamoDBName  oap.ResourceType               = "dynamo-db"
	DynamoDBClass servicecatalog.ClassExternalID = "0dae543c-216b-4a08-87bd-aea7522c0cfd"
	DynamoDBPlan  servicecatalog.PlanExternalID  = "9b59fb3e-56eb-487d-863e-bf831ca4fa3f"
	DynamoPrefix  string                         = "DYNAMO"

	S3       voyager.ResourceType           = "S3"
	S3Name   oap.ResourceType               = "s3"
	S3Class  servicecatalog.ClassExternalID = "a6bf1e70-9bbb-4826-9793-75871cb540f1"
	S3Plan   servicecatalog.PlanExternalID  = "d8eca56a-9634-4e6f-a7c8-47e3bc76bc83"
	S3Prefix string                         = "S3"

	Cfn       voyager.ResourceType           = "CloudFormation"
	CfnName   oap.ResourceType               = "cloudformation"
	CfnClass  servicecatalog.ClassExternalID = "312ebba6-e3df-443f-a151-669a04f0619b"
	CfnPlan   servicecatalog.PlanExternalID  = "8933f0a5-b232-4319-9861-baaccece62fd"
	CfnPrefix string                         = "CF"
)

// All osb-aws-provider resources are 'almost' the same, differing only in the service/plan names,
// what they need passed in the ServiceEnvironment.
var ResourceTypes = map[voyager.ResourceType]wiringplugin.WiringPlugin{
	DynamoDB: wiringutil.StatusAdapter(Resource(DynamoDB, DynamoDBName, DynamoDBClass, DynamoDBPlan, dynamoDbServiceEnvironment, dynamoDbShapes).WireUp),
	S3:       wiringutil.StatusAdapter(Resource(S3, S3Name, S3Class, S3Plan, s3ServiceEnvironment, s3Shapes).WireUp),
	Cfn:      wiringutil.StatusAdapter(Resource(Cfn, CfnName, CfnClass, CfnPlan, CfnServiceEnvironment, cfnShapes).WireUp),
}

func cfnShapes(resource *orch_v1.StateResource, smithResourceName smith_v1.ResourceName, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* externalErr */, bool /* retriableErr */, error) {
	templateName, external, retriable, err := oap.TemplateName(resource.Spec)
	if err != nil {
		return nil, external, retriable, err
	}
	// The AWS broker also returns things like "template-name", "creation-timestamp",
	// and "iamPolicySnippet" which we do not return as environment variables.
	switch templateName {
	case "sns-v1":
		return []wiringplugin.Shape{
			knownshapes.NewSnsSubscribable(smithResourceName),
			knownshapes.NewBindableEnvironmentVariables(smithResourceName, CfnPrefix, map[string]string{
				"TOPICNAME":   "data.TopicName",
				"TOPICARN":    "data.TopicArn",
				"TOPICREGION": "data.TopicRegion",
			}),
			knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
		}, false, false, nil
	case "kinesis-v1":
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResourceName, CfnPrefix, map[string]string{
				"STREAMNAME":   "data.StreamName",
				"STREAMARN":    "data.StreamArn",
				"STREAMREGION": "data.StreamRegion",
			}),
			knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
		}, false, false, nil
	case "elasticsearch-v5":
		fallthrough
	case "elasticsearch-v4":
		fallthrough
	case "elasticsearch-v3":
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResourceName, CfnPrefix, map[string]string{
				"DOMAINARN":      "data.DomainArn",
				"DOMAINENDPOINT": "data.DomainEndpoint",
				"DOMAINREGION":   "data.DomainRegion",
			}),
			knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
		}, false, false, nil
	case "firehose-v1":
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResourceName, CfnPrefix, map[string]string{
				"WRITEREXTERNALROLEARN": "data.WriterExternalRoleArn",
				"DELIVERYROLEARN":       "data.DeliveryRoleArn",
				"S3BUCKETARN":           "data.S3BucketArn",
				"STREAMARN":             "data.StreamArn",
			}),
			knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
		}, false, false, nil
	case "simple-workflow-service-v1":
		return []wiringplugin.Shape{
			knownshapes.NewBindableEnvironmentVariables(smithResourceName, CfnPrefix, map[string]string{
				"DOMAINPREFIX": "data.DomainPrefix",
				"DOMAINREGION": "data.DomainRegion",
			}),
			knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
		}, false, false, nil
	default:
		// There's only a small set of supported Voyager resources in the
		// rps-user-cloudformation repository anyway. All other template types
		// are not documented or supported.  It may be tempting to write a new
		// plugin (or revive the secretenvvar plugin) that goes and just grabs
		// the whole contents of the plugin, but note that this approach gives:
		//
		//  1. An additional layer of indirection that maintains the env var
		//     interface across multiple versions and/or migrations of resources
		//     (i.e. allows adding underscores to things that don't support
		//     underscores, like cloudformation outputs).
		//  2. Smith resolution and validation of references into secrets.
		//  3. Explicit declaration of environment variables implies filtering
		//     of irrelevant outputs (e.g. creationtimestamp) without needing to
		//     construct a blacklist regex. Prevents usages of potentially
		//     internal outputs above the orchestration layer.
		//  4. Structure is defined in the output spec of autowiring (k8s vs
		//     ec2) as opposed to having bespoke logic in the plugin.
		//
		// The secretenvvar approach (originally) needed to:
		//  - pull down all secrets for all resources with the env shape
		//  - run either podsecretenvvar or secretenvvar also passing through a
		//    renameMap and ignoreKeyRegex, and just dump everything into the
		//    environment variables that don't match ignoreKeyRegex.
		return nil, true, false, errors.Errorf("cloudformation template %q is not supported", templateName)
	}
}

func dynamoDbShapes(resource *orch_v1.StateResource, smithResourceName smith_v1.ResourceName, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* externalErr */, bool /* retriableErr */, error) {
	// resource has iamPolicySnippet, creation-timestamp and "table-role",
	// none of which we document or should expose to the user
	return []wiringplugin.Shape{
		knownshapes.NewBindableEnvironmentVariables(smithResourceName, DynamoPrefix, map[string]string{
			"TABLE_NAME":   "data.table-name",
			"TABLE_REGION": "data.table-region",
		}),
		knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
	}, false, false, nil
}

func s3Shapes(resource *orch_v1.StateResource, smithResourceName smith_v1.ResourceName, _ *wiringplugin.WiringContext) ([]wiringplugin.Shape, bool /* externalErr */, bool /* retriableErr */, error) {
	// resource has creation-timestamp, iamPolicySnippet. These are not exposed.
	return []wiringplugin.Shape{
		knownshapes.NewBindableEnvironmentVariables(smithResourceName, S3Prefix, map[string]string{
			"BUCKET_NAME":   "data.bucket-name",
			"BUCKET_PATH":   "data.bucket-path",
			"BUCKET_REGION": "data.bucket-region",
		}),
		knownshapes.NewBindableIamAccessible(smithResourceName, "data.IamPolicySnippet"),
	}, false, false, nil
}

func dynamoDbServiceEnvironment(env *oap.ServiceEnvironment) *oap.ServiceEnvironment {
	return &oap.ServiceEnvironment{
		NotificationEmail: env.NotificationEmail,
		AlarmEndpoints:    env.AlarmEndpoints,
		Tags:              env.Tags,
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
		AlarmEndpoints:       env.AlarmEndpoints,
		Tags:                 env.Tags,
		ServiceSecurityGroup: env.ServiceSecurityGroup,
		NotificationEmail:    env.NotificationEmail,
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
