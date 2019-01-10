package common

import (
	"encoding/json"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/iam"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// TODO: this should change to the plan with fixed API in the long term
	ec2ComputeServiceName = "micros"

	// How we name our Smith resources based on the State resource name
	secretEnvVarFormatString = "%s--secretenvvar" // nolint

	envVarOutputSecretKey = "ec2ComputeEnvVars" // nolint: gosec

	// This regex is applied to renamed reformatted keys which will have a prefix and be uppercase
	envVarIgnoreRegex = `(?i)IamPolicySnippet$`

	// Autowired secret (parametersFrom) input to compute
	bindingOutputRoleARNKey            = "IAMRoleARN"
	bindingOutputInstanceProfileARNKey = "InstanceProfileARN"
	inputParameterEnvVarName           = "secretEnvVars"

	secretEnvVarPluginTypeName = "secretenvvar"

	// special case ec2 -> ec2 dependency relationship
	microsClusterServiceClassExternalName  = "micros"
	microsClusterServicePlanExternalNameV1 = "default-plan"
	microsClusterServicePlanExternalNameV2 = "v2"

	MaximumServiceNameLength = 26

	VoyagerTagValue = "voyager"
	VoyagerTagKey   = "platform"
)

// ec2 v2 plan wiring will implement this
type ConstructComputeParametersFunction func(
	origSpec *runtime.RawExtension,
	iamRoleRef, iamInstProfRef smith_v1.Reference,
	microsServiceName string, stateContext wiringplugin.StateContext) (*runtime.RawExtension, error)

type StateComputeSpec struct {
	RenameEnvVar map[string]string `json:"rename,omitempty"`
}

func generateSecretEnvVarsResource(stateResource *orch_v1.StateResource, computeSpec *StateComputeSpec, dependencyReferences []smith_v1.Reference) (wiringplugin.WiredSmithResource, string, error) {
	// We use objectName for both the smith resource name and the kubernetes metadata name,
	// since there's only one of these per state resource (no possibility of clash).
	objectName := fmt.Sprintf(secretEnvVarFormatString, stateResource.Name)

	secretEnvVarPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: envVarOutputSecretKey,
		OutputJSONKey:   inputParameterEnvVarName,
		RenameEnvVar:    computeSpec.RenameEnvVar,
		IgnoreKeyRegex:  envVarIgnoreRegex,
	})
	if err != nil {
		return wiringplugin.WiredSmithResource{}, "", err
	}

	instanceResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
			Name:       smith_v1.ResourceName(objectName),
			References: dependencyReferences,
			Spec: smith_v1.ResourceSpec{
				Plugin: &smith_v1.PluginSpec{
					Name:       secretEnvVarPluginTypeName,
					ObjectName: objectName,
					Spec:       secretEnvVarPluginSpec,
				},
			},
		},
		Exposed: false,
	}

	return instanceResource, objectName, nil
}

func calculateServiceName(serviceName voyager.ServiceName, resourceName voyager.ResourceName, name string) (string, error) {
	microsServiceName := fmt.Sprintf("%s-%s", serviceName, resourceName)
	if name != "" {
		microsServiceName = name
	}

	if len(microsServiceName) > MaximumServiceNameLength {
		return "", errors.Errorf("generated Micros service name exceeds the limit of 26 characters: %q", microsServiceName)
	}

	return microsServiceName, nil
}

func constructComputeSpec(spec *runtime.RawExtension) (StateComputeSpec, error) {
	var computeSpec StateComputeSpec
	err := json.Unmarshal(spec.Raw, &computeSpec)
	return computeSpec, err
}

func WireUp(microServiceNameInSpec, ec2ComputePlanName string, stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext, constructComputeParameters ConstructComputeParametersFunction) (*wiringplugin.WiringResult, bool, error) {
	dependencies := context.Dependencies

	// Validate ASAP dependencies
	asapDependencyCount := 0
	for _, dep := range context.Dependencies {
		if dep.Type == asapkey.ResourceType {
			// Only allow one asap key dependency per compute
			// so we can use same micros1 env var names and facilitate migration
			if asapDependencyCount++; asapDependencyCount > 1 {
				return nil, false, errors.Errorf("cannot depend on more than one asap key resource")
			}
		}
	}

	// We shouldn't use ServiceName directly here, because we might deploy multiple ec2computes
	// (and each must have a unique servicename). If the user does not specify it instead, we construct...
	// NB Micros will blow up if this is moderately large.
	microsServiceName, err := calculateServiceName(context.StateContext.ServiceName, stateResource.Name, microServiceNameInSpec)
	if err != nil {
		return nil, false, err
	}

	var bindingResources []wiringplugin.WiredSmithResource
	var references []smith_v1.Reference

	for _, dependency := range dependencies {
		bindableShape, found := dependency.Contract.FindShape(knownshapes.BindableEnvironmentVariablesShape)
		if !found {
			return nil, false, errors.Errorf("cannot depend on resource %q of type %q, only dependencies providing shape %q are supported", dependency.Name, dependency.Type, knownshapes.BindableEnvironmentVariablesShape)
		}

		resourceReference := bindableShape.(*knownshapes.BindableEnvironmentVariables).Data.ServiceInstanceName
		bindingResources = append(bindingResources, wiringutil.ConsumerProducerServiceBindingV2(stateResource.Name, dependency.Name, resourceReference, false))
	}

	dependencyReferences := make([]smith_v1.Reference, 0, len(bindingResources))
	for _, res := range bindingResources {
		dependencyReferences = append(dependencyReferences, smith_v1.Reference{
			Resource: res.SmithResource.Name,
		})
	}

	var parametersFrom []sc_v1b1.ParametersFromSource
	computeSpec, err := constructComputeSpec(stateResource.Spec)
	if err != nil {
		return nil, false, errors.Wrap(err, "resource spec could not be decoded as expected spec")
	}

	if len(dependencyReferences) > 0 {
		secretEnvVarsResource, secretEnvVarsResourceMetaName, secretErr := generateSecretEnvVarsResource(stateResource, &computeSpec, dependencyReferences)
		if secretErr != nil {
			return nil, false, secretErr
		}
		ref := sc_v1b1.ParametersFromSource{
			SecretKeyRef: &sc_v1b1.SecretKeyReference{
				Name: secretEnvVarsResourceMetaName,
				Key:  envVarOutputSecretKey,
			},
		}
		parametersFrom = append(parametersFrom, ref)
		references = append(references, smith_v1.Reference{
			Resource: secretEnvVarsResource.SmithResource.Name,
		})
		bindingResources = append(bindingResources, secretEnvVarsResource)
	}

	assumeRoles := []string{context.StateContext.LegacyConfig.DeployerRole}
	managedPolicies := context.StateContext.LegacyConfig.ManagedPolicies
	// TODO we assumed everything generates iam snippets, might want to change this. https://trello.com/c/Tikbwksn/765-iam-plugin-improvements
	iamPluginInstanceSmithResource, err := iam.PluginServiceInstance(iam.EC2ComputeType, stateResource.Name,
		voyager.ServiceName(microsServiceName), true, dependencyReferences, context, managedPolicies, assumeRoles)
	if err != nil {
		return nil, false, err
	}

	iamPluginBindingSmithResource := iam.ServiceBinding(stateResource.Name, iamPluginInstanceSmithResource.SmithResource.Name)

	iamRoleRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.SmithResource.Name, bindingOutputRoleARNKey),
		Resource: iamPluginBindingSmithResource.SmithResource.Name,
		Path:     fmt.Sprintf("data.%s", bindingOutputRoleARNKey),
		Example:  "arn:aws:iam::123456789012:role/path/role",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	iamInstProfRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.SmithResource.Name, bindingOutputInstanceProfileARNKey),
		Resource: iamPluginBindingSmithResource.SmithResource.Name,
		Path:     fmt.Sprintf("data.%s", bindingOutputInstanceProfileARNKey),
		Example:  "arn:aws:iam::123456789012:instance-profile/path/Webserver",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	references = append(references, iamRoleRef, iamInstProfRef)

	serviceInstanceSpec, err := constructComputeParameters(stateResource.Spec, iamRoleRef, iamInstProfRef, microsServiceName, context.StateContext)
	if err != nil {
		return nil, false, err
	}
	computeResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
			Name:       wiringutil.ServiceInstanceResourceName(stateResource.Name),
			References: references,
			Spec: smith_v1.ResourceSpec{
				Object: &sc_v1b1.ServiceInstance{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceInstance",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: wiringutil.ServiceInstanceMetaName(stateResource.Name),
					},
					Spec: sc_v1b1.ServiceInstanceSpec{
						PlanReference: sc_v1b1.PlanReference{
							ClusterServiceClassExternalName: ec2ComputeServiceName,
							ClusterServicePlanExternalName:  ec2ComputePlanName,
						},
						Parameters:     serviceInstanceSpec,
						ParametersFrom: parametersFrom,
					},
				},
			},
		},
		Exposed: true,
	}

	// Wire Result
	smithResources := append(bindingResources, iamPluginInstanceSmithResource, iamPluginBindingSmithResource, computeResource)

	result := &wiringplugin.WiringResult{
		Resources: smithResources,
	}

	return result, false, nil

}
