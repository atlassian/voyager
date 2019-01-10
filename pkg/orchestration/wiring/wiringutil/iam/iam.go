package iam

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	iam_plugin "github.com/atlassian/voyager/pkg/execution/plugins/atlassian/iamrole"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/aws"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	EC2ComputeType  = iam_plugin.EC2ComputeType
	KubeComputeType = iam_plugin.KubeComputeType

	iamPluginTypeName smith_v1.PluginName = "iamrole"

	// prefix starts with `-` to disambiguate these two ServiceBindings:
	// ServiceBinding for compute X depending on ServiceInstance produced by Resource Y
	//     resource name: {X}--{Y}--binding
	//     meta name    : {X}--{Y}
	// ServiceBinding for compute X depending on ServiceInstance produced by IAM plugin for compute X
	//     resource name: {X}---iamrole
	//     meta name    : {X}---iamrole-v2
	//
	// See how meta names would clash if Resource Y was named "iamrole" if there was no extra `-`?
	// See iam_test for the test.
	namePostfix = "-iamrole"
)

func PluginServiceInstance(computeType iam_plugin.ComputeType, resourceName voyager.ResourceName, serviceName voyager.ServiceName, createInstanceProfile bool, dependencyReferences []smith_v1.Reference,
	context *wiringplugin.WiringContext, managedPolicies, assumeRoles []string) (smith_v1.Resource, error) {

	iamRoleSpecJSONMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&iam_plugin.Spec{
		OAPResourceName:       string(resourceName) + namePostfix,
		ServiceName:           serviceName,
		CreateInstanceProfile: createInstanceProfile,
		ManagedPolicies:       managedPolicies,
		AssumeRoles:           assumeRoles,
		ServiceEnvironment:    *aws.CfnServiceEnvironment(oap.MakeServiceEnvironmentFromContext(context)),
		ComputeType:           computeType,
	})
	if err != nil {
		return smith_v1.Resource{}, err
	}

	instanceResource := smith_v1.Resource{
		Name:       wiringutil.ResourceNameWithPostfix(resourceName, namePostfix),
		References: dependencyReferences,
		Spec: smith_v1.ResourceSpec{
			Plugin: &smith_v1.PluginSpec{
				Name:       iamPluginTypeName,
				ObjectName: wiringutil.MetaNameWithPostfix(resourceName, namePostfix),
				Spec:       iamRoleSpecJSONMap,
			},
		},
	}

	return instanceResource, nil
}

func ServiceBinding(compute voyager.ResourceName, iamPluginServiceInstance smith_v1.ResourceName) smith_v1.Resource {
	return wiringutil.ResourceInternalServiceBinding(compute, iamPluginServiceInstance, namePostfix)
}
