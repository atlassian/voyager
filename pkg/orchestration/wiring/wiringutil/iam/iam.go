package iam

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	iam_plugin "github.com/atlassian/voyager/pkg/execution/plugins/atlassian/iamrole"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/aws"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
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

	// This is just the local reference name
	dependencyNamePostfix = "iamrole"
)

type ResourceWithIamAccessibleBinding struct {
	ResourceName               voyager.ResourceName
	BindableIamAccessibleShape knownshapes.BindableIamAccessible
	BindingName                smith_v1.ResourceName
}

func PluginServiceInstance(computeType iam_plugin.ComputeType, stateResourceName voyager.ResourceName,
	serviceName voyager.ServiceName, createInstanceProfile bool, iamShapedResources []ResourceWithIamAccessibleBinding,
	context *wiringplugin.WiringContext, managedPolicies, assumeRoles []string, vpc *oap.VPCEnvironment) (smith_v1.Resource, error) {

	dependencyReferences := make([]smith_v1.Reference, 0, len(iamShapedResources))
	iamPolicyDocumentRefs := make(map[string]string, len(iamShapedResources))
	for _, iamShapedResource := range iamShapedResources {
		ref := iamShapedResource.BindableIamAccessibleShape.Data.IAMPolicySnippet.ToReference(
			wiringutil.ReferenceName(iamShapedResource.BindingName, dependencyNamePostfix),
			iamShapedResource.BindingName,
		)
		dependencyReferences = append(dependencyReferences, ref)
		iamPolicyDocumentRefs[string(iamShapedResource.ResourceName)] = ref.Ref()
	}

	iamRoleSpecJSONMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&iam_plugin.Spec{
		OAPResourceName:       string(stateResourceName) + namePostfix,
		ServiceName:           serviceName,
		CreateInstanceProfile: createInstanceProfile,
		ManagedPolicies:       managedPolicies,
		AssumeRoles:           assumeRoles,
		ServiceEnvironment:    *aws.CfnServiceEnvironment(oap.MakeServiceEnvironmentFromContext(context, vpc)),
		ComputeType:           computeType,
		PolicySnippets:        iamPolicyDocumentRefs,
	})
	if err != nil {
		return smith_v1.Resource{}, err
	}

	instanceResource := smith_v1.Resource{
		Name:       wiringutil.ResourceNameWithPostfix(stateResourceName, namePostfix),
		References: dependencyReferences,
		Spec: smith_v1.ResourceSpec{
			Plugin: &smith_v1.PluginSpec{
				Name:       iamPluginTypeName,
				ObjectName: wiringutil.MetaNameWithPostfix(stateResourceName, namePostfix),
				Spec:       iamRoleSpecJSONMap,
			},
		},
	}

	return instanceResource, nil
}

func ServiceBinding(compute voyager.ResourceName, iamPluginServiceInstance smith_v1.ResourceName) smith_v1.Resource {
	return wiringutil.ResourceInternalServiceBinding(compute, iamPluginServiceInstance, namePostfix)
}
