package iamrole

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"sort"
	"text/template"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Arbitrary template name we pass to OAP, forms part of CFN stack name
const iamRoleTemplateName = "iam"

// We just use '%s' for now to generate the template.
// TODO directly referencing Truman-Dev managed policy here at the moment. Unclear how we want
// to manage this in future. Also, we miss the stuff from default.json here (notably
// SetInstanceHealth - the other stuff such as s3 secret extraction we don't know about in voyager).
const roleTemplate = `
{
  "AWSTemplateFormatVersion": "2010-09-09",
  "Description": "Voyager Smith Plugin generated IAM policy",
  "Outputs": {
    "IAMRole": {
      "Value": {"Ref": "IAMRole"}
    },
    "IAMRoleARN": {
      "Value": {"Fn::GetAtt" : ["IAMRole", "Arn"]}
    }
{{- if .createInstanceProfile}},
    "InstanceProfile": {
      "Value": {"Ref": "InstanceProfile"}
    },
    "InstanceProfileARN": {
      "Value": {"Fn::GetAtt" : ["InstanceProfile", "Arn"]}
    }
{{- end}}
  },
  "Resources": {
    "IAMRole": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "AssumeRolePolicyDocument": {
          "Version": "2012-10-17",
          "Statement": {{.assumeRoles}}
        },
        "ManagedPolicyArns": {{.managedPolicies}},
        "Policies": {{.policies}}
      }
    }
{{- if .createInstanceProfile}},
    "InstanceProfile": {
      "Type": "AWS::IAM::InstanceProfile",
      "Properties": {
        "Roles": [
          {
            "Ref": "IAMRole"
          }
        ]
      }
    }
{{- end}}
  }
}
`
const (
	indent                         = "  "
	prettyPrintIndent              = "        "
	assumeRoleStatementPrintIndent = "          "
)

const (
	cloudformationServiceID = "312ebba6-e3df-443f-a151-669a04f0619b"
	cloudformationPlanID    = "8933f0a5-b232-4319-9861-baaccece62fd"
)

var defaultEC2ComputeAssumeRoleStatement = IamAssumeRoleStatement{
	Effect: "Allow",
	Principal: IamAssumeRolePrincipal{Service: []string{
		"ec2.amazonaws.com",
		"lambda.amazonaws.com",
		"autoscaling.amazonaws.com",
		"redshift.amazonaws.com",
	}},
	Action: "sts:AssumeRole",
}

func defaultJSON(serviceName voyager.ServiceName) IamPolicy {
	var condition json.RawMessage = []byte(fmt.Sprintf(`
					{
						"StringEquals": {
						  "autoscaling:ResourceTag/micros_service_id": "%s"
						}
                  	}`, serviceName))

	return IamPolicy{
		PolicyName: "default.json",
		PolicyDocument: IamPolicyDocument{
			Version: "2012-10-17",
			Statement: []IamPolicyStatement{
				{
					Effect: "Allow",
					Action: []string{
						"s3:GetObjectVersion",
						"s3:GetObject",
						"s3:ListBucket",
					},
					Resource: []string{
						"arn:aws:s3:::config-store.*.atl-inf.io",
						fmt.Sprintf("arn:aws:s3:::config-store.*.atl-inf.io/%s/*", serviceName),
					},
				},
				{
					Action: []string{
						"s3:PutObject",
						"s3:GetObject",
						"s3:GetObjectVersion",
					},
					Effect: "Allow",
					Resource: []string{
						fmt.Sprintf("arn:aws:s3:::micros-runcmd-*/%s/*", serviceName),
						fmt.Sprintf("arn:aws:s3:::access-logs.*/service-logs/%s/*", serviceName),
					},
				},
				{
					Action: []string{
						"s3:ListBucket",
					},
					Effect: "Allow",
					Resource: []string{
						"arn:aws:s3:::micros-runcmd-*",
						fmt.Sprintf("arn:aws:s3:::micros-runcmd-*/%s/*", serviceName),
					},
				},
				{
					Effect: "Allow",
					Action: []string{
						"sns:Publish",
						"sns:Subscribe",
						"sns:Unsubscribe",
						"sqs:*",
					},
					Resource: []string{
						fmt.Sprintf("arn:aws:sns:*:*:stk-evts--%.37s--*", serviceName),
						fmt.Sprintf("arn:aws:sqs:*:*:stk-evts--%.37s--*", serviceName),
					},
				},
				{
					Effect: "Allow",
					Action: []string{
						"autoscaling:SetInstanceHealth",
					},
					Resource: []string{
						"*",
					},
					Condition: &condition,
				},
			},
		},
	}
}

func generateRoleInstance(spec *Spec) (*sc_v1b1.ServiceInstance, error) {
	policyDocuments := make([]IamPolicyDocument, 0, len(spec.PolicySnippets))
	for resourceName, policySnippet := range spec.PolicySnippets {
		policy := IamPolicyDocument{}
		if err := json.Unmarshal([]byte(policySnippet), &policy); err != nil {
			return nil, errors.Wrapf(err, "failed to extract IAM policy from the resource %q", resourceName)
		}

		if err := validatePolicy(&policy); err != nil {
			return nil, errors.Wrapf(err, "invalid policy emitted by %q", resourceName)
		}

		policyDocuments = append(policyDocuments, policy)
	}

	policy, err := combineIamPolicies(policyDocuments)
	if err != nil {
		return nil, err
	}

	managedPolicies := make([]string, 0)
	if spec.ManagedPolicies != nil {
		managedPolicies = spec.ManagedPolicies
	}

	parameters, err := constructCloudFormationPayload(spec.ComputeType, spec.OAPResourceName, policy, spec.ServiceName,
		spec.CreateInstanceProfile, managedPolicies, spec.AssumeRoles, spec.ServiceEnvironment)
	if err != nil {
		return nil, err
	}

	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			Parameters: parameters,
			ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
				Name: cloudformationServiceID,
			},
			ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
				Name: cloudformationPlanID,
			},
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalID: cloudformationServiceID,
				ClusterServicePlanExternalID:  cloudformationPlanID,
			},
		},
	}, nil
}

func constructCloudFormationPayload(computeType ComputeType, oapResourceName string, policy IamPolicy, serviceID voyager.ServiceName, createInstanceProfile bool, managedPolicies, assumeRoles []string, env oap.ServiceEnvironment) (*runtime.RawExtension, error) {

	iamPolicies := make([]IamPolicy, 0, 2) // should not be nil to avoid serializing it as `null`

	// only add default.json policy if compute type is EC2ComputeType
	if computeType == EC2ComputeType && serviceID != "" {
		iamPolicies = append(iamPolicies, defaultJSON(serviceID))
	}

	if len(policy.PolicyDocument.Statement) > 0 {
		iamPolicies = append(iamPolicies, policy)
	}

	policyBytes, err := json.MarshalIndent(iamPolicies, prettyPrintIndent, "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal policy")
	}
	managedPolicyBytes, err := json.Marshal(managedPolicies)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal managed policies")
	}

	iamAssumeRoleStatementBytes, err := generateIamAssumeRoleStatements(computeType, assumeRoles)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal assume role statements")
	}

	temp, err := buildTemplate(policyBytes, managedPolicyBytes, iamAssumeRoleStatementBytes, createInstanceProfile)
	if err != nil {
		return nil, err
	}

	attributes, err := util.ToRawExtension(&CfnAttributes{
		Template:     iamRoleTemplateName,
		TemplateBody: temp,
	})
	if err != nil {
		return nil, err
	}

	return util.ToRawExtension(&oap.ServiceInstanceSpec{
		ServiceName: serviceID,
		Resource: oap.RPSResource{
			Type:       "cloudformation",
			Name:       oapResourceName,
			Attributes: attributes.Raw,
		},
		Environment: env,
	})
}

func combineIamPolicies(policies []IamPolicyDocument) (IamPolicy, error) {
	rolePolicy, err := mergePolicies(policies)
	if err != nil {
		return IamPolicy{}, errors.Wrap(err, "unmergeable policy")
	}
	return IamPolicy{
		PolicyName:     "voyager-merge",
		PolicyDocument: rolePolicy,
	}, nil
}

func makeStatementKey(statement *IamPolicyStatement) (string, error) {
	type StatementKey struct {
		Condition *json.RawMessage
		Action    []string
		NotAction []string
	}
	statementKey := StatementKey{
		Condition: statement.Condition,
	}
	// Copy so we can sort non-destructively
	statementKey.Action = append(statementKey.Action, statement.Action...)
	statementKey.NotAction = append(statementKey.NotAction, statement.NotAction...)
	sort.Strings(statementKey.Action)
	sort.Strings(statementKey.NotAction)

	var key bytes.Buffer
	err := gob.NewEncoder(&key).Encode(statementKey)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return key.String(), nil
}

func mergePolicies(policies []IamPolicyDocument) (IamPolicyDocument, error) {
	// This not only puts all statements in one policy, it also merges
	// statements that have the same actions/conditions.
	statementsByKey := make(map[string]IamPolicyStatement)
	for _, policy := range policies {
		for _, statement := range policy.Statement {
			statementKey, err := makeStatementKey(&statement)
			if err != nil {
				return IamPolicyDocument{}, err
			}
			for _, resource := range statement.Resource {
				statementByKey, present := statementsByKey[statementKey]
				if !present {
					statementByKey = IamPolicyStatement{
						Effect:    "Allow",
						Action:    statement.Action,
						NotAction: statement.NotAction,
						Condition: statement.Condition,
					}
				}
				statementByKey.Resource = append(statementByKey.Resource, resource)
				statementsByKey[statementKey] = statementByKey
			}
		}
	}

	// Now we need to sort things so we have a stable output
	keys := make([]string, 0, len(statementsByKey))
	for k := range statementsByKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	statements := make([]IamPolicyStatement, 0, len(statementsByKey))
	for _, k := range keys {
		statementByKey := statementsByKey[k]
		sort.Strings(statementByKey.Resource)
		statements = append(statements, statementByKey)
	}

	return IamPolicyDocument{
		Version:   "2012-10-17",
		Statement: statements,
	}, nil
}

func validatePolicy(policy *IamPolicyDocument) error {
	for _, statement := range policy.Statement {
		if statement.Effect == "Deny" {
			return errors.New("Effect:Deny not supported due to merging complexity")
		}
		if statement.Principal != nil || statement.NotPrincipal != nil {
			return errors.New("Principal not supported since policy attached to resource not role")
		}
		if len(statement.NotResource) > 0 {
			return errors.New("NotResource not supported due to dangerous effects (and merge code)")
		}
	}

	return nil
}

func generateIamAssumeRoleStatements(computeType ComputeType, assumeRoleNames []string) ([]byte, error) {
	assumeRoles := make([]IamAssumeRoleStatement, 0, len(assumeRoleNames)+1)

	// only add defaultEC2ComputeAssumeRoleStatement if compute type is EC2ComputeType
	if computeType == EC2ComputeType {
		assumeRoles = append(assumeRoles, defaultEC2ComputeAssumeRoleStatement)
	}

	for _, roleName := range assumeRoleNames {
		assumeRoles = append(assumeRoles, IamAssumeRoleStatement{
			Effect:    "Allow",
			Principal: IamAssumeRolePrincipal{AWS: roleName},
			Action:    "sts:AssumeRole",
		})
	}

	assumeRolesBytes, err := json.MarshalIndent(assumeRoles, assumeRoleStatementPrintIndent, indent)

	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal assume role statements")
	}

	return assumeRolesBytes, nil
}

func buildTemplate(policyBytes, managedPolicyBytes, iamAssumeRoleStatementBytes []byte, createInstanceProfile bool) (string, error) {
	params := map[string]interface{}{
		"policies":              string(policyBytes),
		"managedPolicies":       string(managedPolicyBytes),
		"createInstanceProfile": createInstanceProfile,
		"assumeRoles":           string(iamAssumeRoleStatementBytes),
	}

	buf := &bytes.Buffer{}
	t, err := template.New("").Parse(roleTemplate)
	if err != nil {
		return "", errors.Wrap(err, "failed to create template")
	}
	if err := t.Execute(buf, params); err != nil {
		return "", errors.Wrap(err, "failed to write template")
	}

	return buf.String(), nil
}
