package common

import (
	"encoding/json"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
	"github.com/atlassian/voyager/pkg/execution/plugins/generic/secretplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/compute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/iam"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	nameElement      = "name"
	metadataElement  = "metadata"
	metadataNamePath = "metadata.name"

	// TODO: this should change to the plan with fixed API in the long term
	ec2ComputeServiceName = "micros"

	// How we name our Smith resources based on the State resource name
	secretEnvVarNamePostfix = "secretenvvar"
	secretPluginNamePostfix = "secretplugin"

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

func generateSecretResource(compute voyager.ResourceName, envVars map[string]string, dependencyReferences []smith_v1.Reference) (smith_v1.Resource, error) {
	// if this is for pods then we just have data
	envVarJSONString, err := json.Marshal(map[string]map[string]string{
		inputParameterEnvVarName: envVars,
	})
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}

	secretPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretplugin.Spec{
		Data: map[string][]byte{
			envVarOutputSecretKey: envVarJSONString,
		},
	})
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}

	instanceResource := smith_v1.Resource{
		Name:       wiringutil.ResourceNameWithPostfix(compute, secretPluginNamePostfix),
		References: dependencyReferences,
		Spec: smith_v1.ResourceSpec{
			Plugin: &smith_v1.PluginSpec{
				Name:       secretplugin.PluginName,
				ObjectName: wiringutil.MetaNameWithPostfix(compute, secretPluginNamePostfix),
				Spec:       secretPluginSpec,
			},
		},
	}

	return instanceResource, nil
}

func generateSecretEnvVarsResource(compute voyager.ResourceName, renameEnvVar map[string]string, dependencyReferences []smith_v1.Reference) (smith_v1.Resource, error) {
	// We use objectName for both the smith resource name and the kubernetes metadata name,
	// since there's only one of these per state resource (no possibility of clash).
	objectName := wiringutil.MetaNameWithPostfix(compute, secretEnvVarNamePostfix)

	secretEnvVarPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.Spec{
		OutputSecretKey: envVarOutputSecretKey,
		OutputJSONKey:   inputParameterEnvVarName,
		RenameEnvVar:    renameEnvVar,
		IgnoreKeyRegex:  envVarIgnoreRegex,
	})
	if err != nil {
		return smith_v1.Resource{}, err
	}

	instanceResource := smith_v1.Resource{
		Name:       smith_v1.ResourceName(objectName),
		References: dependencyReferences,
		Spec: smith_v1.ResourceSpec{
			Plugin: &smith_v1.PluginSpec{
				Name:       secretEnvVarPluginTypeName,
				ObjectName: objectName,
				Spec:       secretEnvVarPluginSpec,
			},
		},
	}

	return instanceResource, nil
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

	var bindingResources []smith_v1.Resource
	var bindingResult []compute.BindingResult
	var references []smith_v1.Reference

	for _, dependency := range dependencies {
		bindableShape, found, err := knownshapes.FindBindableEnvironmentVariablesShape(dependency.Contract.Shapes)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, errors.Errorf("cannot depend on resource %q of type %q, only dependencies providing shape %q are supported", dependency.Name, dependency.Type, knownshapes.BindableEnvironmentVariablesShape)
		}

		resourceReference := bindableShape.Data.ServiceInstanceName
		bindingResource := wiringutil.ConsumerProducerServiceBinding(stateResource.Name, dependency.Name, resourceReference)
		bindingResources = append(bindingResources, bindingResource)
		bindingResult = append(bindingResult, compute.BindingResult{
			ResourceName:            dependency.Name,
			BindableEnvVarShape:     *bindableShape,
			CreatedBindingFromShape: bindingResource,
		})
	}

	// filter out the list of dependencyReferences - this will be used
	dependencyReferences := make([]smith_v1.Reference, 0, len(bindingResult))
	shouldUseSecretPlugin := true
	for _, res := range bindingResult {
		ref := smith_v1.Reference{
			Resource: res.CreatedBindingFromShape.Name,
		}
		dependencyReferences = append(dependencyReferences, ref)
		if res.BindableEnvVarShape.Data.Vars == nil {
			shouldUseSecretPlugin = false
		}
	}

	// We use the SecretEnvVar plugin if any of our inputs don't provide vars
	// (deprecated method). Otherwise, we can safely use the secret plugin.
	var parametersFrom []sc_v1b1.ParametersFromSource
	computeSpec, err := constructComputeSpec(stateResource.Spec)
	if err != nil {
		return nil, false, errors.Wrap(err, "resource spec could not be decoded as expected spec")
	}

	if len(bindingResult) > 0 {
		var secretResource smith_v1.Resource
		var err error
		if shouldUseSecretPlugin {
			secretRefs, envVars, err := compute.GenerateEnvVars(computeSpec.RenameEnvVar, bindingResult)
			if err != nil {
				return nil, false, err
			}

			secretResource, err = generateSecretResource(stateResource.Name, envVars, secretRefs)
			if err != nil {
				return nil, false, err
			}
		} else {
			secretResource, err = generateSecretEnvVarsResource(stateResource.Name, computeSpec.RenameEnvVar, dependencyReferences)
			if err != nil {
				return nil, false, err
			}
		}
		secretRef := smith_v1.Reference{
			Name:     wiringutil.ReferenceName(secretResource.Name, metadataElement, nameElement),
			Resource: secretResource.Name,
			Path:     metadataNamePath,
		}
		ref := sc_v1b1.ParametersFromSource{
			SecretKeyRef: &sc_v1b1.SecretKeyReference{
				Name: secretRef.Ref(),
				Key:  envVarOutputSecretKey,
			},
		}
		parametersFrom = append(parametersFrom, ref)
		references = append(references, secretRef)
		bindingResources = append(bindingResources, secretResource)
	}

	assumeRoles := []string{context.StateContext.LegacyConfig.DeployerRole}
	managedPolicies := context.StateContext.LegacyConfig.ManagedPolicies
	// TODO we assumed everything generates iam snippets, might want to change this. https://trello.com/c/Tikbwksn/765-iam-plugin-improvements
	iamPluginInstanceSmithResource, err := iam.PluginServiceInstance(iam.EC2ComputeType, stateResource.Name,
		voyager.ServiceName(microsServiceName), true, dependencyReferences, context, managedPolicies, assumeRoles)
	if err != nil {
		return nil, false, err
	}

	iamPluginBindingSmithResource := iam.ServiceBinding(stateResource.Name, iamPluginInstanceSmithResource.Name)

	iamRoleRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.Name, bindingOutputRoleARNKey),
		Resource: iamPluginBindingSmithResource.Name,
		Path:     fmt.Sprintf("data.%s", bindingOutputRoleARNKey),
		Example:  "arn:aws:iam::123456789012:role/path/role",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	iamInstProfRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.Name, bindingOutputInstanceProfileARNKey),
		Resource: iamPluginBindingSmithResource.Name,
		Path:     fmt.Sprintf("data.%s", bindingOutputInstanceProfileARNKey),
		Example:  "arn:aws:iam::123456789012:instance-profile/path/Webserver",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	references = append(references, iamRoleRef, iamInstProfRef)

	serviceInstanceSpec, err := constructComputeParameters(stateResource.Spec, iamRoleRef, iamInstProfRef, microsServiceName, context.StateContext)
	if err != nil {
		return nil, false, err
	}
	computeResource := smith_v1.Resource{
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
	}

	// Wire Result
	smithResources := append(bindingResources, iamPluginInstanceSmithResource, iamPluginBindingSmithResource, computeResource)

	result := &wiringplugin.WiringResult{
		Resources: smithResources,
	}

	return result, false, nil

}
