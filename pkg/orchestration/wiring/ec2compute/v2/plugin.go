package v2

import (
	"encoding/json"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/execution/plugins/generic/secretplugin"
	compute_common "github.com/atlassian/voyager/pkg/orchestration/wiring/compute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/compute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/iam"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "EC2Compute"

	ec2ComputePlanName = "v2"

	nameElement      = "name"
	metadataElement  = "metadata"
	metadataNamePath = "metadata.name"

	// TODO: this should change to the plan with fixed API in the long term
	ec2ComputeServiceName = "micros"

	// How we name our Smith resources based on the State resource name
	secretPluginNamePostfix = "secret"

	envVarOutputSecretKey = "ec2ComputeEnvVars" // nolint: gosec

	// Autowired secret (parametersFrom) input to compute
	bindingOutputRoleARNKey            = "IAMRoleARN"
	bindingOutputInstanceProfileARNKey = "InstanceProfileARN"
	inputParameterEnvVarName           = "secretEnvVars"

	MaximumServiceNameLength = 26
)

// HACK: Some tags the EC2 provider doesn't like, because it wants to
// set them itself... (NB handles business_unit/resource_owner separately)
// We only really worry about the tags that we're likely to set here
// (it's ok if the user errors out from the provider).
var forbiddenTags = map[string]struct{}{
	"environment":      {},
	"environment_type": {},
	"service_name":     {},
}

type userInputSpec struct {
	Service service `json:"service"`
}

type Docker struct {
	EnvVars map[string]string `json:"envVars"`
}

// fields that the auto wiring function manipulates
type partialSpec struct {
	Service        service               `json:"service"`
	Location       voyager.Location      `json:"location"`
	EC2            ec2Iam                `json:"ec2"`
	Tags           map[string]string     `json:"tags"`
	Notifications  notifications         `json:"notifications"`
	SecretEnvVars  map[string]string     `json:"secretEnvVars,omitempty"`
	Docker         Docker                `json:"docker"`
	AlarmEndpoints []oap.MicrosAlarmSpec `json:"alarmEndpoints"`
}

type service struct {
	ID              string `json:"id"`
	LoggingID       string `json:"loggingId"`
	SsamAccessLevel string `json:"ssamAccessLevel"`
}

type ec2Iam struct {
	IamRoleArn            string `json:"iamRoleArn"`
	IamInstanceProfileArn string `json:"iamInstanceProfileArn"`
}

type notifications struct {
	Email string `json:"email"`
}

// restrictedParameters contains the parts of the output compute spec users cannot set
// because we automatically generate them and don't allow overrides.
type restrictedParameters struct {
	Location voyager.Location `json:"location"`
	// SecretEnvVars is a pointer so we can do == comparisons against an empty object
	// (otherwise we will fail to compare maps).
	SecretEnvVars *map[string]string `json:"secretEnvVars,omitempty"`
	EC2           ec2Iam             `json:"ec2"`
}

// ec2 v2 plan wiring will implement this
type ConstructComputeParametersFunction func(
	origSpec *runtime.RawExtension,
	iamRoleRef, iamInstProfRef smith_v1.Reference,
	microsServiceName string, stateContext wiringplugin.StateContext) (*runtime.RawExtension, bool /* external */, bool /* retriable */, error)

type StateComputeSpec struct {
	RenameEnvVar map[string]string `json:"rename,omitempty"`
}

func constructComputeParameters(origSpec *runtime.RawExtension, iamRoleRef, iamInstProfRef smith_v1.Reference, microsServiceName string, stateContext wiringplugin.StateContext) (*runtime.RawExtension, bool /* external */, bool /* retriable */, error) {
	// The user shouldn't be setting anything in our 'restrictedParameters', since
	// _we_ control it. So let's make sure they're not and fail ASAP.
	var parametersCheck restrictedParameters
	if err := json.Unmarshal(origSpec.Raw, &parametersCheck); err != nil {
		return nil, false, false, errors.Wrap(err, "can't unmarshal state spec into JSON object")
	}
	if parametersCheck != (restrictedParameters{}) {
		// User provided something weird in the spec
		return nil, true, false, errors.Errorf("at least one autowired value not empty: %+v", parametersCheck)
	}

	// generate partialSpec

	var partialSpecData partialSpec
	// service param
	partialSpecData.Service = service{
		ID:              microsServiceName,
		LoggingID:       stateContext.ServiceProperties.LoggingID,
		SsamAccessLevel: stateContext.ServiceProperties.SSAMAccessLevel,
	}

	// --- location param
	partialSpecData.Location = stateContext.Location

	// --- ec2 param
	partialSpecData.EC2 = ec2Iam{
		IamRoleArn:            iamRoleRef.Ref(),
		IamInstanceProfileArn: iamInstProfRef.Ref(),
	}

	// --- tags params
	partialSpecData.Tags = make(map[string]string, len(stateContext.Tags))
	for k, v := range stateContext.Tags {
		if _, forbidden := forbiddenTags[string(k)]; !forbidden {
			partialSpecData.Tags[string(k)] = v
		}
	}

	// --- notificationProp params
	notificationProp := stateContext.ServiceProperties.Notifications
	partialSpecData.Notifications = notifications{
		Email: notificationProp.Email,
	}
	partialSpecData.AlarmEndpoints = oap.PagerdutyAlarmEndpoints(
		notificationProp.PagerdutyEndpoint.CloudWatch, notificationProp.LowPriorityPagerdutyEndpoint.CloudWatch)

	// --- default ASAP public key repo env vars
	sharedDefaultEnvVars := compute_common.GetSharedDefaultEnvVars(stateContext.Location)
	partialSpecData.Docker.EnvVars = make(map[string]string, len(sharedDefaultEnvVars))
	for _, v := range sharedDefaultEnvVars {
		partialSpecData.Docker.EnvVars[v.Name] = v.Value
	}

	// convert partialSpec to map
	var partialSpecMap map[string]interface{}
	partialSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&partialSpecData)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	var finalSpec map[string]interface{}
	if err = json.Unmarshal(origSpec.Raw, &finalSpec); err != nil {
		return nil, false, false, errors.Wrap(err, "failed to parse user spec")
	}

	wiringutil.StripJSONFields(finalSpec, StateComputeSpec{})

	// merge user spec and partial spec
	finalSpec, err = wiringutil.Merge(finalSpec, partialSpecMap)
	if err != nil {
		return nil, false, false, err
	}

	rawExtension, err := util.ToRawExtension(finalSpec)
	if err != nil {
		return nil, false, false, err
	}

	return rawExtension, false, false, nil
}

func New(developerRole func(location voyager.Location) []string,
	managedPolicies func(location voyager.Location) []string,
	vpc func(location voyager.Location) *oap.VPCEnvironment) *WiringPlugin {
	return &WiringPlugin{
		DeveloperRole:   developerRole,
		ManagedPolicies: managedPolicies,
		VPC:             vpc,
	}
}

type WiringPlugin struct {
	DeveloperRole   func(location voyager.Location) []string
	ManagedPolicies func(location voyager.Location) []string
	VPC             func(location voyager.Location) *oap.VPCEnvironment
}

func (p *WiringPlugin) WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if stateResource.Type != ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", stateResource.Type),
		}
	}

	if stateResource.Spec == nil {
		return &wiringplugin.WiringResultFailure{
			Error: errors.New("resource spec must be provided"),
		}
	}

	userInput := userInputSpec{}
	if err := json.Unmarshal(stateResource.Spec.Raw, &userInput); err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Wrap(err, "failed to parse user spec"),
		}
	}

	return p.wireUp(userInput.Service.ID, ec2ComputePlanName, stateResource, context, constructComputeParameters)
}

func generateSecretResource(compute voyager.ResourceName, envVars map[string]string, dependencyReferences []smith_v1.Reference) (smith_v1.Resource, error) {
	secretPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretplugin.Spec{
		JSONData: map[string]interface{}{
			envVarOutputSecretKey: map[string]map[string]string{
				inputParameterEnvVarName: envVars,
			},
		},
	})
	if err != nil {
		return smith_v1.Resource{}, errors.WithStack(err)
	}

	instanceResource := smith_v1.Resource{
		Name:       wiringutil.ResourceName(compute, secretPluginNamePostfix),
		References: dependencyReferences,
		Spec: smith_v1.ResourceSpec{
			Plugin: &smith_v1.PluginSpec{
				Name:       secretplugin.PluginName,
				ObjectName: wiringutil.MetaName(compute, secretPluginNamePostfix),
				Spec:       secretPluginSpec,
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

func (p *WiringPlugin) wireUp(microServiceNameInSpec, ec2ComputePlanName string, stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext, constructComputeParameters ConstructComputeParametersFunction) wiringplugin.WiringResult {
	dependencies := context.Dependencies

	if err := compute.ValidateASAPDependencies(context); err != nil {
		// ASAP dependencies are wrong, user issue
		return &wiringplugin.WiringResultFailure{
			Error:           err,
			IsExternalError: true,
		}
	}

	// We shouldn't use ServiceName directly here, because we might deploy multiple ec2computes
	// (and each must have a unique servicename). If the user does not specify it instead, we construct...
	// NB Micros will blow up if this is moderately large.
	microsServiceName, err := calculateServiceName(context.StateContext.ServiceName, stateResource.Name, microServiceNameInSpec)
	if err != nil {
		// Service Name invalid is because of spec issue
		return &wiringplugin.WiringResultFailure{
			IsExternalError: true,
			Error:           err,
		}
	}

	var bindingResources []smith_v1.Resource
	var resourcesWithEnvVarBindings []compute.ResourceWithEnvVarBinding
	var resourcesWithIamAccessibleBindings []iam.ResourceWithIamAccessibleBinding
	var references []smith_v1.Reference

	for _, dependency := range dependencies {
		bindableEnvVarShape, envVarFound, err := knownshapes.FindBindableEnvironmentVariablesShape(dependency.Contract.Shapes)
		if err != nil {
			// this error is because the wiring return an invalid shape or had duplicate shapes - this is an internal error (code issue)
			return &wiringplugin.WiringResultFailure{
				Error: err,
			}
		}
		if !envVarFound {
			return &wiringplugin.WiringResultFailure{
				Error:           errors.Errorf("cannot depend on resource %q, only dependencies providing shape %q are supported", dependency.Name, knownshapes.BindableEnvironmentVariablesShape),
				IsExternalError: true,
			}
		}

		resourceReference := bindableEnvVarShape.Data.ServiceInstanceName.ToReference()
		bindingResource := wiringutil.ConsumerProducerServiceBinding(stateResource.Name, dependency.Name, resourceReference)
		bindingResources = append(bindingResources, bindingResource)
		resourcesWithEnvVarBindings = append(resourcesWithEnvVarBindings, compute.ResourceWithEnvVarBinding{
			ResourceName:        dependency.Name,
			BindableEnvVarShape: *bindableEnvVarShape,
			BindingName:         bindingResource.Name,
		})

		// We also depend on BindableIamAccessible shape
		bindableIamAccessibleShape, iamFound, err := knownshapes.FindBindableIamAccessibleShape(dependency.Contract.Shapes)
		if err != nil {
			// this error is because the wiring return an invalid shape or had duplicate shapes - this is an internal error (code issue)
			return &wiringplugin.WiringResultFailure{
				Error:           err,
				IsExternalError: true,
			}
		}
		if iamFound {
			var iamBindingResourceName smith_v1.ResourceName
			iamResourceReference := bindableIamAccessibleShape.Data.ServiceInstanceName.ToReference()
			// Reuse the binding if the service instance name reference is the same, otherwise
			// we'll need to do another binding
			if wiringutil.IsSameTarget(iamResourceReference, resourceReference) {
				iamBindingResourceName = bindingResource.Name
			} else {
				iamBindingResource := wiringutil.ConsumerProducerServiceBinding(stateResource.Name, dependency.Name, iamResourceReference)
				bindingResources = append(bindingResources, iamBindingResource)
				iamBindingResourceName = iamBindingResource.Name
			}
			resourcesWithIamAccessibleBindings = append(resourcesWithIamAccessibleBindings, iam.ResourceWithIamAccessibleBinding{
				ResourceName:               dependency.Name,
				BindingName:                iamBindingResourceName,
				BindableIamAccessibleShape: *bindableIamAccessibleShape,
			})
		}
	}

	var parametersFrom []sc_v1b1.ParametersFromSource
	var computeSpec StateComputeSpec
	err = json.Unmarshal(stateResource.Spec.Raw, &computeSpec)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Wrap(err, "resource spec could not be decoded as expected spec"),
		}
	}

	if len(resourcesWithEnvVarBindings) > 0 {
		secretRefs, envVars, err := compute.GenerateEnvVars(computeSpec.RenameEnvVar, resourcesWithEnvVarBindings)
		if err != nil {
			// This is more likely a user error due to renames as environment variables are namespaced
			// per resource and resource type.
			return &wiringplugin.WiringResultFailure{
				Error:           err,
				IsExternalError: true,
			}
		}

		secretResource, err := generateSecretResource(stateResource.Name, envVars, secretRefs)
		if err != nil {
			return &wiringplugin.WiringResultFailure{
				Error: err,
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

	assumeRoles := p.DeveloperRole(context.StateContext.Location)
	managedPolicies := p.ManagedPolicies(context.StateContext.Location)

	// The only things that generate IamSnippets are the things that have the correct shape
	iamPluginInstanceSmithResource, err := iam.PluginServiceInstance(
		iam.EC2ComputeType,
		stateResource.Name,
		voyager.ServiceName(microsServiceName),
		true,
		resourcesWithIamAccessibleBindings,
		context,
		managedPolicies,
		assumeRoles,
		p.VPC(context.StateContext.Location),
	)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
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

	serviceInstanceSpec, external, retriable, err := constructComputeParameters(stateResource.Spec, iamRoleRef, iamInstProfRef, microsServiceName, context.StateContext)
	if err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:            err,
			IsExternalError:  external,
			IsRetriableError: retriable,
		}
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

	return &wiringplugin.WiringResultSuccess{
		Resources: smithResources,
	}
}
