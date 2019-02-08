package k8scompute

import (
	"fmt"
	"regexp"
	"sort"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/execution/plugins/generic/secretplugin"
	"github.com/atlassian/voyager/pkg/k8s"
	compute_common "github.com/atlassian/voyager/pkg/orchestration/wiring/compute"
	apik8scompute "github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/compute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/iam"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	policy_v1 "k8s.io/api/policy/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	nameElement      = "name"
	metadataElement  = "metadata"
	metadataNamePath = "metadata.name"

	resourceNameLabel          = "resourceName"
	stateNameLabel             = "stateName"
	kittBusinessUnitAnnotation = "atlassian.com/business_unit"
	loggingIDAnnotation        = "atlassian.com/logging_id"
	resourceOwnerAnnotation    = "atlassian.com/resource_owner"
	kube2iamAnnotation         = "iam.amazonaws.com/role"

	serviceAccountPostFix   = "svcacc"
	secretPluginNamePostfix = "secret"
	pdbPostfix              = "pdb"
	hpaPostfix              = "hpa"
	bindingOutputRoleARNKey = "IAMRoleARN"

	defaultPodDisruptionBudget = "30%"

	// Default environment variable names
	awsRegionKey   = "MICROS_AWS_REGION"
	envTypeKey     = "MICROS_ENVTYPE"
	serviceNameKey = "MICROS_SERVICE"
)

var (
	imageValidatorRegex = regexp.MustCompile(`^.+[:@].+$`)
)

func validateSpec(spec *Spec) error {
	errorList := util.NewErrorList()

	if err := validateContainerDockerImage(spec); err != nil {
		errorList.Add(err)
	}

	if err := validateScaling(spec.Scaling); err != nil {
		errorList.Add(err)
	}

	if errorList.HasErrors() {
		return errorList
	}

	return nil
}

func validateContainerDockerImage(spec *Spec) error {
	for _, container := range spec.Containers {
		//the pattern is for checking if @ or : is specified in the image
		if !imageValidatorRegex.MatchString(container.Image) {
			return errors.Errorf("tag or digest needs to be specified for image %q", container.Image)
		}
	}
	return nil
}

func validateScaling(s Scaling) error {
	// we only need to validate when min & max are both positive;
	// if one or more are 0, it means we provision a deployment with no HPA
	if s.MinReplicas > 0 &&
		s.MaxReplicas > 0 &&
		s.MinReplicas > s.MaxReplicas {
		return errors.Errorf("MaxReplicas (%d) must be greater than MinReplicas (%d)", s.MaxReplicas, s.MinReplicas)
	}

	return nil
}

// WireUp is the main autowiring function for the K8SCompute resource, building a native kube deployment and HPA
func WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if resource.Type != apik8scompute.ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", resource.Type),
		}
	}

	if err := compute.ValidateASAPDependencies(context); err != nil {
		return &wiringplugin.WiringResultFailure{
			Error:           err,
			IsExternalError: true,
		}
	}

	// Parse spec and apply defaults
	spec := &Spec{}
	if err := resource.SpecIntoTyped(spec); err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	// Apply the defaults from the resource state
	// Defaults are defined in the formation layer
	if err := spec.ApplyDefaults(resource.Defaults); err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: err,
		}
	}

	if err := validateSpec(spec); err != nil {
		// completely a user error
		return &wiringplugin.WiringResultFailure{
			Error:           err,
			IsExternalError: true,
		}
	}
	// Prepare environment variables
	var envDefault []core_v1.EnvVar
	var envFrom []core_v1.EnvFromSource
	var smithResources []smith_v1.Resource
	var resourceWithEnvVarBindings []compute.ResourceWithEnvVarBinding
	var resourcesWithIamAccessibleBindings []iam.ResourceWithIamAccessibleBinding
	references := make([]smith_v1.Reference, 0, len(context.Dependencies))

	for _, dep := range context.Dependencies {
		bindableEnvVarShape, found, err := knownshapes.FindBindableEnvironmentVariablesShape(dep.Contract.Shapes)
		if err != nil {
			// this error is because the wiring return an invalid shape or had duplicate shapes - this is an internal error (code issue)
			return &wiringplugin.WiringResultFailure{
				Error: err,
			}
		}
		if !found {
			return &wiringplugin.WiringResultFailure{
				Error:           errors.Errorf("cannot depend on resource %q, only dependencies providing shape %q are supported", dep.Name, knownshapes.BindableEnvironmentVariablesShape),
				IsExternalError: true,
			}
		}

		resourceReference := bindableEnvVarShape.Data.ServiceInstanceName
		binding := wiringutil.ConsumerProducerServiceBinding(resource.Name, dep.Name, resourceReference)
		smithResources = append(smithResources, binding)
		resourceWithEnvVarBindings = append(resourceWithEnvVarBindings, compute.ResourceWithEnvVarBinding{
			ResourceName:        dep.Name,
			BindableEnvVarShape: *bindableEnvVarShape,
			BindingName:         binding.Name,
		})

		// We also depend on BindableIamAccessible shape
		bindableIamAccessibleShape, iamFound, err := knownshapes.FindBindableIamAccessibleShape(dep.Contract.Shapes)
		if err != nil {
			// this error is because the wiring return an invalid shape or had duplicate shapes - this is an internal error (code issue)
			return &wiringplugin.WiringResultFailure{
				Error: err,
			}
		}
		if iamFound {
			var iamBindingResource smith_v1.Resource
			iamResourceReference := bindableIamAccessibleShape.Data.ServiceInstanceName
			// Reuse the binding if the service instance name is the same, otherwise
			// we'll need to do another binding
			if iamResourceReference == resourceReference {
				iamBindingResource = binding
			} else {
				iamBindingResource = wiringutil.ConsumerProducerServiceBinding(resource.Name, dep.Name, iamResourceReference)
				smithResources = append(smithResources, iamBindingResource)
			}
			resourcesWithIamAccessibleBindings = append(resourcesWithIamAccessibleBindings, iam.ResourceWithIamAccessibleBinding{
				ResourceName:               dep.Name,
				BindingName:                iamBindingResource.Name,
				BindableIamAccessibleShape: *bindableIamAccessibleShape,
			})
		}
	}

	var iamRoleRef *smith_v1.Reference

	// Reference environment variables retrieved from ServiceBinding objects
	if len(resourceWithEnvVarBindings) > 0 {
		secretRefs, envVars, err := compute.GenerateEnvVars(spec.RenameEnvVar, resourceWithEnvVarBindings)
		if err != nil {
			// any errors in generating environment variables is more likely to be a user error
			return &wiringplugin.WiringResultFailure{
				Error:           err,
				IsExternalError: true,
			}
		}

		secretResource, err := generateSecretResource(resource.Name, envVars, secretRefs)
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
		falseObj := false
		envFromSource := core_v1.EnvFromSource{
			SecretRef: &core_v1.SecretEnvSource{
				LocalObjectReference: core_v1.LocalObjectReference{
					Name: secretRef.Ref(),
				},
				Optional: &falseObj,
			},
		}

		envFrom = append(envFrom, envFromSource)
		references = append(references, secretRef)
		smithResources = append(smithResources, secretResource)

		iamPluginInstanceSmithResource, err := iam.PluginServiceInstance(iam.KubeComputeType, resource.Name,
			context.StateContext.ServiceName, false, resourcesWithIamAccessibleBindings, context, []string{}, buildKube2iamRoles(context))
		if err != nil {
			return &wiringplugin.WiringResultFailure{
				Error: err,
			}
		}

		iamPluginBindingSmithResource := iam.ServiceBinding(resource.Name, iamPluginInstanceSmithResource.Name)

		iamRoleRef = &smith_v1.Reference{
			Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.Name, bindingOutputRoleARNKey),
			Resource: iamPluginBindingSmithResource.Name,
			Path:     fmt.Sprintf("data.%s", bindingOutputRoleARNKey),
			Example:  "arn:aws:iam::123456789012:role/path/role",
			Modifier: smith_v1.ReferenceModifierBindSecret,
		}
		references = append(references, *iamRoleRef)
		smithResources = append(smithResources, iamPluginInstanceSmithResource, iamPluginBindingSmithResource)
	}

	serviceAccountResource := smith_v1.Resource{
		Name: wiringutil.ResourceNameWithPostfix(resource.Name, serviceAccountPostFix),
		Spec: smith_v1.ResourceSpec{
			Object: &core_v1.ServiceAccount{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.ServiceAccountKind,
					APIVersion: core_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.MetaNameWithPostfix(resource.Name, serviceAccountPostFix),
				},
				ImagePullSecrets: []core_v1.LocalObjectReference{{Name: apik8scompute.DockerImagePullName}},
			},
		},
	}
	smithResources = append(smithResources, serviceAccountResource)

	serviceAccountNameRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(serviceAccountResource.Name, metadataElement, nameElement),
		Resource: serviceAccountResource.Name,
		Path:     metadataNamePath,
	}
	references = append(references, serviceAccountNameRef)

	// Shared default env vars
	// - Including ASAP public key servers, as all containers
	//   should know where to get the public keys
	//   regardless if they're using ASAP or not
	envDefault = append(envDefault, compute_common.GetSharedDefaultEnvVars(context.StateContext.Location)...)

	// Add Micros provided defaults
	envDefault = append(envDefault, buildDefaultEnvVars(context.StateContext)...)

	// always bind to the common secret, it's OK if it doesn't exist
	trueVar := true
	commonEnvFrom := core_v1.EnvFromSource{
		SecretRef: &core_v1.SecretEnvSource{
			LocalObjectReference: core_v1.LocalObjectReference{
				Name: apik8scompute.CommonSecretName,
			},
			Optional: &trueVar,
		},
	}
	envFrom = append(envFrom, commonEnvFrom)

	// prepare containers
	containers := buildContainers(spec, envDefault, envFrom)

	labelMap := map[string]string{
		stateNameLabel:    context.StateMeta.Name,
		resourceNameLabel: string(resource.Name),
	}

	affinity := buildAffinity(labelMap)

	podSpec := buildPodSpec(containers, serviceAccountNameRef.Ref(), affinity)

	// Add pod disruption budget
	pdbSpec := buildPodDisruptionBudgetSpec(labelMap)
	pdb := smith_v1.Resource{
		Name: wiringutil.ResourceNameWithPostfix(resource.Name, pdbPostfix),
		Spec: smith_v1.ResourceSpec{
			Object: &policy_v1.PodDisruptionBudget{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.PodDisruptionBudgetKind,
					APIVersion: policy_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.MetaNameWithPostfix(resource.Name, pdbPostfix),
				},
				Spec: pdbSpec,
			},
		},
	}
	smithResources = append(smithResources, pdb)

	// The kube deployment object spec
	deploymentSpec := buildDeploymentSpec(context, spec, podSpec, labelMap, iamRoleRef)

	// The final wired deployment object
	deployment := smith_v1.Resource{
		Name:       wiringutil.ResourceName(resource.Name),
		References: references,
		Spec: smith_v1.ResourceSpec{
			Object: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.DeploymentKind,
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: wiringutil.MetaName(resource.Name),
				},
				Spec: deploymentSpec,
			},
		},
	}

	smithResources = append(smithResources, deployment)

	deploymentNameRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(deployment.Name, metadataElement, nameElement),
		Resource: deployment.Name,
		Path:     metadataNamePath,
	}

	// 0 value for replicas indicates we don't need an HPA
	if spec.Scaling.MinReplicas > 0 && spec.Scaling.MaxReplicas > 0 {
		hpaSpec := buildHorizontalPodAutoscalerSpec(spec, deploymentNameRef.Ref())

		// The final wired HPA object
		hpa := smith_v1.Resource{
			Name:       wiringutil.ResourceNameWithPostfix(resource.Name, hpaPostfix),
			References: []smith_v1.Reference{deploymentNameRef},
			Spec: smith_v1.ResourceSpec{
				Object: &autoscaling_v2b1.HorizontalPodAutoscaler{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       k8s.HorizontalPodAutoscalerKind,
						APIVersion: autoscaling_v2b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: wiringutil.MetaNameWithPostfix(resource.Name, hpaPostfix),
					},
					Spec: hpaSpec,
				},
			},
		}

		smithResources = append(smithResources, hpa)
	}

	return &wiringplugin.WiringResultSuccess{
		Contract: wiringplugin.ResourceContract{
			Shapes: []wiringplugin.Shape{
				knownshapes.NewSetOfPodsSelectableByLabels(deployment.Name, labelMap),
			},
		},
		Resources: smithResources,
	}
}

func generateSecretResource(compute voyager.ResourceName, envVars map[string]string, dependencyReferences []smith_v1.Reference) (smith_v1.Resource, error) {
	secretPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretplugin.Spec{
		Data: envVars,
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

func buildPodSpec(containers []core_v1.Container, serviceAccountName string, affinity *core_v1.Affinity) core_v1.PodSpec {
	var terminationGracePeriodSeconds int64 = 30
	return core_v1.PodSpec{
		Containers:         containers,
		ServiceAccountName: serviceAccountName,
		Affinity:           affinity,

		// field with default values
		DNSPolicy:                     "ClusterFirst",
		RestartPolicy:                 "Always",
		SchedulerName:                 "default-scheduler",
		SecurityContext:               &core_v1.PodSecurityContext{},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
	}
}

func buildDeploymentSpec(context *wiringplugin.WiringContext, spec *Spec, podSpec core_v1.PodSpec, labelMap map[string]string, iamRoleRef *smith_v1.Reference) apps_v1.DeploymentSpec {
	progressDeadlineSeconds := int32(600)
	revisionHistoryLimit := int32(0)

	annotations := map[string]string{
		kittBusinessUnitAnnotation: context.StateContext.ServiceProperties.BusinessUnit,
		loggingIDAnnotation:        context.StateContext.ServiceProperties.LoggingID,
		resourceOwnerAnnotation:    context.StateContext.ServiceProperties.ResourceOwner,
	}

	if iamRoleRef != nil {
		annotations[kube2iamAnnotation] = iamRoleRef.Ref()
	}

	// Set replicas to the scaling min, ensure there is at least 1 replica
	replicas := spec.Scaling.MinReplicas
	if replicas == 0 {
		replicas = 1
	}

	return apps_v1.DeploymentSpec{
		Selector: &meta_v1.LabelSelector{
			MatchLabels: labelMap,
		},
		Replicas: &replicas,
		Template: core_v1.PodTemplateSpec{
			ObjectMeta: meta_v1.ObjectMeta{
				Labels:      labelMap,
				Annotations: annotations,
			},
			Spec: podSpec,
		},
		// fields which default values
		Strategy: apps_v1.DeploymentStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &apps_v1.RollingUpdateDeployment{
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "25%",
				},
				MaxSurge: &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "25%",
				},
			},
		},
		ProgressDeadlineSeconds: &progressDeadlineSeconds,
		RevisionHistoryLimit:    &revisionHistoryLimit,
	}
}

func buildContainers(spec *Spec, envDefault []core_v1.EnvVar, envFrom []core_v1.EnvFromSource) []core_v1.Container {
	containers := make([]core_v1.Container, 0, len(spec.Containers))

	for _, container := range spec.Containers {
		containers = append(containers, container.ToKubeContainer(envDefault, envFrom))
	}

	return containers
}

func buildDefaultEnvVars(context wiringplugin.StateContext) []core_v1.EnvVar {
	return []core_v1.EnvVar{
		{
			Name:  awsRegionKey,
			Value: string(context.Location.Region),
		},
		{
			Name:  envTypeKey,
			Value: string(context.Location.EnvType),
		},
		{
			Name:  serviceNameKey,
			Value: string(context.ServiceName),
		},
	}
}

func buildAffinity(labelMap map[string]string) *core_v1.Affinity {
	return &core_v1.Affinity{
		PodAntiAffinity: buildAntiAffinity(labelMap),
	}
}

func buildAntiAffinity(labelMap map[string]string) *core_v1.PodAntiAffinity {
	// Convert the labelMap into a a slice of LabelSelectorRequirements
	matchExpressions := make([]meta_v1.LabelSelectorRequirement, 0, len(labelMap))

	// Iterate over sorted keys because otherwise this becomes untestable
	keys := make([]string, 0, len(labelMap))
	for k := range labelMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		matchExpressions = append(matchExpressions,
			meta_v1.LabelSelectorRequirement{
				Key:      k,
				Operator: meta_v1.LabelSelectorOpIn,
				Values:   []string{labelMap[k]},
			},
		)
	}

	// Create WeightedPodAffinityTerms to configure antiaffinity to distibute
	// the app to different zones, and then nodes (where possible)
	podAffinityTerms := []core_v1.WeightedPodAffinityTerm{
		core_v1.WeightedPodAffinityTerm{
			Weight: 75,
			PodAffinityTerm: core_v1.PodAffinityTerm{
				LabelSelector: &meta_v1.LabelSelector{
					MatchExpressions: matchExpressions,
				},
				TopologyKey: k8s.LabelZoneFailureDomain,
			},
		},
		core_v1.WeightedPodAffinityTerm{
			Weight: 50,
			PodAffinityTerm: core_v1.PodAffinityTerm{
				LabelSelector: &meta_v1.LabelSelector{
					MatchExpressions: matchExpressions,
				},
				TopologyKey: k8s.LabelHostname,
			},
		},
	}

	return &core_v1.PodAntiAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: podAffinityTerms,
	}
}

func buildPodDisruptionBudgetSpec(labelMap map[string]string) policy_v1.PodDisruptionBudgetSpec {
	return policy_v1.PodDisruptionBudgetSpec{
		MinAvailable: &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: defaultPodDisruptionBudget,
		},
		Selector: &meta_v1.LabelSelector{
			MatchLabels: labelMap,
		},
	}
}

func buildHorizontalPodAutoscalerSpec(spec *Spec, deploymentName string) autoscaling_v2b1.HorizontalPodAutoscalerSpec {
	metrics := make([]autoscaling_v2b1.MetricSpec, len(spec.Scaling.Metrics))

	for i, m := range spec.Scaling.Metrics {
		metrics[i] = m.ToKubeMetric()
	}

	return autoscaling_v2b1.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscaling_v2b1.CrossVersionObjectReference{
			APIVersion: apps_v1.SchemeGroupVersion.String(),
			Kind:       k8s.DeploymentKind,
			Name:       deploymentName,
		},
		MinReplicas: &spec.Scaling.MinReplicas,
		MaxReplicas: spec.Scaling.MaxReplicas,
		Metrics:     metrics,
	}
}

func buildKube2iamRoles(context *wiringplugin.WiringContext) []string {
	account := context.StateContext.ClusterConfig.Kube2iamAccount
	region := context.StateContext.Location.Region
	env := context.StateContext.ClusterConfig.KittClusterEnv

	nodeRole := fmt.Sprintf("arn:aws:iam::%s:role/%s.paas-%s_node-role", account, region, env)
	controlerRole := fmt.Sprintf("arn:aws:iam::%s:role/controller-role-%s.paas-%s", account, region, env)

	return []string{nodeRole, controlerRole}
}
