package k8scompute

import (
	"fmt"
	"regexp"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/execution/plugins/atlassian/secretenvvar"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/iam"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/atlassian/voyager/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	serviceAccountPostFix         = "svcacc"
	podSecretEnvVarNamePostfix    = "podsecretenvvar"
	hpaPostfix                    = "hpa"
	podSecretEnvVarPluginTypeName = "podsecretenvvar"
	bindingOutputRoleARNKey       = "IAMRoleARN"
	envVarIgnoreRegex             = `^IamPolicySnippet$`
)

var (
	instanceGK          = schema.GroupKind{Group: sc_v1b1.GroupName, Kind: k8s.ServiceInstanceKind}
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
func WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool /*retriable*/, error) {
	if resource.Type != apik8scompute.ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", resource.Type)
	}

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

	// Parse spec and apply defaults
	spec := &Spec{}
	if err := resource.SpecIntoTyped(spec); err != nil {
		return nil, false, err
	}

	// Apply the defaults from the resource state
	// Defaults are defined in the formation layer
	if err := spec.ApplyDefaults(resource.Defaults); err != nil {
		return nil, false, err
	}

	if err := validateSpec(spec); err != nil {
		return nil, false, err
	}
	// Prepare environment variables
	var envFrom []core_v1.EnvFromSource
	var smithResources []wiringplugin.WiredSmithResource
	var bindingResources []wiringplugin.WiredSmithResource
	references := make([]smith_v1.Reference, 0, len(context.Dependencies))

	for _, dep := range context.Dependencies {
		bound := false

		if bindableShape, ok := dep.Contract.FindShape(knownshapes.BindableEnvironmentVariablesShape); ok {
			resourceReference := bindableShape.(*knownshapes.BindableEnvironmentVariables).Data.ServiceInstanceName
			// We don't want anything that depends on compute to see our bindings - exposed: false
			binding := wiringutil.ConsumerProducerServiceBindingV2(resource.Name, dep.Name, resourceReference, false)
			smithResources = append(smithResources, binding)
			bindingResources = append(bindingResources, binding)
			continue
		}

		// DEPRECATED: binding using raw SmithResources, use resource contracts instead
		if len(dep.SmithResources) > 0 {
			for _, dependencyObj := range dep.SmithResources {
				if dependencyObj.Spec.Object == nil {
					// TODO support plugins
					continue
				}
				dependencyObjGVK := dependencyObj.Spec.Object.GetObjectKind().GroupVersionKind()
				switch dependencyObjGVK.GroupKind() {
				case instanceGK:
					bound = true
					// We don't want anything that depends on compute to see our bindings - exposed: false
					binding := wiringutil.ConsumerProducerServiceBinding(resource.Name, dep.Name, dependencyObj.Name, false)
					smithResources = append(smithResources, binding)
					bindingResources = append(bindingResources, binding)
				}
			}
		}

		// check if we wired/bound dependency in at least one way
		if !bound {
			return nil, false, errors.Errorf("cannot depend on resource %q of type %q. Only ServiceInstance type is supported", dep.Name, dep.Type)
		}
	}

	var iamRoleRef *smith_v1.Reference

	// Reference environment variables retrieved from ServiceBinding objects
	if len(bindingResources) > 0 {
		bindingReferences := make([]smith_v1.Reference, 0, len(bindingResources))
		for _, res := range bindingResources {
			bindingReferences = append(bindingReferences, smith_v1.Reference{
				Resource: res.SmithResource.Name,
			})
		}

		secretEnvVarsResource, err := generateSecretEnvVarsResource(resource.Name, spec, bindingReferences)
		if err != nil {
			return nil, false, err
		}
		secretEnvVarRef := smith_v1.Reference{
			Name:     wiringutil.ReferenceName(secretEnvVarsResource.SmithResource.Name, metadataElement, nameElement),
			Resource: secretEnvVarsResource.SmithResource.Name,
			Path:     metadataNamePath,
		}
		falseObj := false
		envFromSource := core_v1.EnvFromSource{
			SecretRef: &core_v1.SecretEnvSource{
				LocalObjectReference: core_v1.LocalObjectReference{
					Name: secretEnvVarRef.Ref(),
				},
				Optional: &falseObj,
			},
		}
		envFrom = append(envFrom, envFromSource)
		references = append(references, secretEnvVarRef)
		smithResources = append(smithResources, secretEnvVarsResource)

		iamPluginInstanceSmithResource, err := iam.PluginServiceInstance(iam.KubeComputeType, resource.Name,
			context.StateContext.ServiceName, false, bindingReferences, context, []string{}, buildKube2iamRoles(context))
		if err != nil {
			return nil, false, err
		}

		iamPluginBindingSmithResource := iam.ServiceBinding(resource.Name, iamPluginInstanceSmithResource.SmithResource.Name)

		iamRoleRef = &smith_v1.Reference{
			Name:     wiringutil.ReferenceName(iamPluginBindingSmithResource.SmithResource.Name, bindingOutputRoleARNKey),
			Resource: iamPluginBindingSmithResource.SmithResource.Name,
			Path:     fmt.Sprintf("data.%s", bindingOutputRoleARNKey),
			Example:  "arn:aws:iam::123456789012:role/path/role",
			Modifier: smith_v1.ReferenceModifierBindSecret,
		}
		references = append(references, *iamRoleRef)
		smithResources = append(smithResources, iamPluginInstanceSmithResource, iamPluginBindingSmithResource)
	}

	serviceAccountResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
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
		},
	}
	smithResources = append(smithResources, serviceAccountResource)

	serviceAccountNameRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(serviceAccountResource.SmithResource.Name, metadataElement, nameElement),
		Resource: serviceAccountResource.SmithResource.Name,
		Path:     metadataNamePath,
	}
	references = append(references, serviceAccountNameRef)

	// default env vars for containers
	var envDefault []core_v1.EnvVar

	// ASAP public key servers
	// we want every container to know where to get the public keys
	// regardless if they're using ASAP or not
	envDefault = append(envDefault, asapkey.GetPublicKeyRepoEnvVars(context.StateContext.Location)...)

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

	podSpec := buildPodSpec(containers, serviceAccountNameRef.Ref())

	// The kube deployment object spec
	deploymentSpec := buildDeploymentSpec(context, spec, podSpec, labelMap, iamRoleRef)

	// The final wired deployment object
	deployment := wiringplugin.WiredSmithResource{
		Exposed: true,
		SmithResource: smith_v1.Resource{
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
		},
	}

	smithResources = append(smithResources, deployment)

	deploymentNameRef := smith_v1.Reference{
		Name:     wiringutil.ReferenceName(deployment.SmithResource.Name, metadataElement, nameElement),
		Resource: deployment.SmithResource.Name,
		Path:     metadataNamePath,
	}

	// 0 value for replicas indicates we don't need an HPA
	if spec.Scaling.MinReplicas > 0 && spec.Scaling.MaxReplicas > 0 {
		hpaSpec := buildHorizontalPodAutoscalerSpec(spec, deploymentNameRef.Ref())

		// The final wired HPA object
		hpa := wiringplugin.WiredSmithResource{
			Exposed: true,
			SmithResource: smith_v1.Resource{
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
			},
		}

		smithResources = append(smithResources, hpa)
	}

	result := &wiringplugin.WiringResult{
		Resources: smithResources,
	}

	return result, false, nil
}

func generateSecretEnvVarsResource(compute voyager.ResourceName, spec *Spec, dependencyReferences []smith_v1.Reference) (wiringplugin.WiredSmithResource, error) {
	secretEnvVarPluginSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secretenvvar.PodSpec{
		IgnoreKeyRegex: envVarIgnoreRegex,
		RenameEnvVar:   spec.RenameEnvVar,
	})
	if err != nil {
		return wiringplugin.WiredSmithResource{}, err
	}

	// We use objectName for both the smith resource name and the kubernetes metadata name,
	// since there's only one of these per state resource (no possibility of clash).
	instanceResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
			Name:       wiringutil.ResourceNameWithPostfix(compute, podSecretEnvVarNamePostfix),
			References: dependencyReferences,
			Spec: smith_v1.ResourceSpec{
				Plugin: &smith_v1.PluginSpec{
					Name:       podSecretEnvVarPluginTypeName,
					ObjectName: wiringutil.MetaNameWithPostfix(compute, podSecretEnvVarNamePostfix),
					Spec:       secretEnvVarPluginSpec,
				},
			},
		},
		Exposed: false,
	}

	return instanceResource, nil
}

func buildPodSpec(containers []core_v1.Container, serviceAccountName string) core_v1.PodSpec {
	var terminationGracePeriodSeconds int64 = 30
	return core_v1.PodSpec{
		Containers:         containers,
		ServiceAccountName: serviceAccountName,

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
