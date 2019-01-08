package kubeingress

import (
	"encoding/json"
	"fmt"
	"strconv"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/internaldns"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/knownshapes"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ResourceType voyager.ResourceType = "KubeIngress"

	kittClusterEnvPlayground  = "playground"
	kittClusterEnvIntegration = "integration"

	// these are intentionally different so that if the kitt cluster env names change
	// we don't unintentionally change the DNS names (which we own and are separate)
	envTypePlayground  = "playground"
	envTypeIntegration = "integration"

	kittIngressTypeAnnotation = "voyager.atl-paas.net/ingressType"
	contourTimeoutAnnotation  = "contour.heptio.com/request-timeout"

	// The port is hardcoded for now, see https://sdog.jira-dev.com/browse/MICROS-6451
	servicePort     = 8080
	clusterHostPath = "k8s.atl-paas.net"

	servicePostfix = "service"
	ingressPostfix = "ingress"

	defaultContourTimeout = 60
)

// WireUp is the main autowiring function for KubeIngress
func WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool /*retriable*/, error) {

	// Fail if the resource type is wrong
	if resource.Type != ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", resource.Type)
	}

	deploymentName, deploymentLabels, err := extractKubeComputeDependencyDetails(context.Dependencies)
	if err != nil {
		return nil, false, err
	}

	var smithResources []smith_v1.Resource

	// Build the Service
	serviceResource := buildServiceResource(smith_v1.ResourceName(deploymentName), deploymentLabels, resource, context)
	smithResources = append(smithResources, serviceResource)

	// Build the Ingress
	ingressResource, err := buildIngressResource(serviceResource.Name, resource, context)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed building ingress resource")
	}
	smithResources = append(smithResources, ingressResource)

	result := &wiringplugin.WiringResult{
		Resources: smithResources,
	}

	return result, false, nil
}

// buildServiceResource constructs the Kube Service object
func buildServiceResource(deploymentName smith_v1.ResourceName, selectorLabels map[string]string, resource *orch_v1.StateResource, context *wiringplugin.WiringContext) smith_v1.Resource {
	serviceName := string(resource.Name) + "-" + servicePostfix

	serviceSpec := core_v1.ServiceSpec{
		Ports: []core_v1.ServicePort{
			{
				Name:       "http",
				Port:       servicePort,
				TargetPort: intstr.FromInt(servicePort),
				Protocol:   "TCP",
			},
		},
		Selector:        selectorLabels,
		Type:            core_v1.ServiceTypeClusterIP,
		SessionAffinity: "None",
	}

	serviceResource := smith_v1.Resource{
		Name: smith_v1.ResourceName(serviceName),
		References: []smith_v1.Reference{
			{
				Resource: deploymentName,
			},
		},
		Spec: smith_v1.ResourceSpec{
			Object: &core_v1.Service{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.ServiceKind,
					APIVersion: core_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: serviceName,
				},
				Spec: serviceSpec,
			},
		},
	}

	return serviceResource
}

func buildIngressResourceFromSpec(serviceName smith_v1.ResourceName, resourceName voyager.ResourceName, timeout int, context *wiringplugin.WiringContext) (smith_v1.Resource, error) {
	var ingressRules []ext_v1b1.IngressRule
	ingressName := wiringutil.ResourceNameWithPostfix(resourceName, ingressPostfix)
	ingressRuleValue := ext_v1b1.IngressRuleValue{
		HTTP: &ext_v1b1.HTTPIngressRuleValue{
			Paths: []ext_v1b1.HTTPIngressPath{
				{
					Path: "/",
					Backend: ext_v1b1.IngressBackend{
						ServiceName: string(serviceName),
						ServicePort: intstr.FromInt(servicePort),
					},
				},
			},
		},
	}

	// default rule
	hostname := buildIngressHostName(resourceName, context.StateContext)
	ingressRules = append(ingressRules, ext_v1b1.IngressRule{
		Host:             hostname,
		IngressRuleValue: ingressRuleValue,
	})

	// internalDNS rules
	for _, dependency := range context.Dependants {
		if dependency.Type == internaldns.ResourceType {
			var internalDNSSpec internaldns.Spec
			if err := json.Unmarshal(dependency.Resource.Spec.Raw, &internalDNSSpec); err != nil {
				return smith_v1.Resource{}, err
			}
			for _, alias := range internalDNSSpec.Aliases {
				ingressRules = append(ingressRules, ext_v1b1.IngressRule{
					Host:             alias.Name,
					IngressRuleValue: ingressRuleValue,
				})
			}
		}
	}

	ingressSpec := ext_v1b1.IngressSpec{
		Rules: ingressRules,
	}

	ingressResource := smith_v1.Resource{
		Name: ingressName,
		References: []smith_v1.Reference{
			{
				Resource: serviceName,
			},
		},
		Spec: smith_v1.ResourceSpec{
			Object: &ext_v1b1.Ingress{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       k8s.IngressKind,
					APIVersion: ext_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: string(ingressName),
					Annotations: map[string]string{
						kittIngressTypeAnnotation: "private",
						// incoming requests pass through ALB and Envoy
						// KITT will set ALB's timeout to 5 minutes, so that it is higher than any reasonable value supplied by the user
						contourTimeoutAnnotation: strconv.Itoa(timeout) + "s",
					},
				},
				Spec: ingressSpec,
			},
		},
	}

	return ingressResource, nil
}

// buildIngressResource constructs the Kube / KITT Ingress object
// with a default rule, plus all alias rules from dependant internalDNS resources
func buildIngressResource(serviceResourceName smith_v1.ResourceName, resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (smith_v1.Resource, error) {
	var timeout = defaultContourTimeout

	if resource.Defaults != nil {
		var rawDefaultsSpec Spec
		if err := json.Unmarshal(resource.Defaults.Raw, &rawDefaultsSpec); err != nil {
			return smith_v1.Resource{}, errors.WithStack(err)
		}
		if rawDefaultsSpec.TimeoutSeconds != nil {
			timeout = *rawDefaultsSpec.TimeoutSeconds
		}
	}

	if resource.Spec != nil {
		var rawIngressSpec Spec
		if err := json.Unmarshal(resource.Spec.Raw, &rawIngressSpec); err != nil {
			return smith_v1.Resource{}, errors.WithStack(err)
		}

		if rawIngressSpec.TimeoutSeconds != nil {
			timeout = *rawIngressSpec.TimeoutSeconds

			if timeout < 1 || timeout > 300 {
				return smith_v1.Resource{}, errors.Errorf(
					"ingress timeout must be between one second and five minutes (was given %d seconds)", timeout)
			}
		}
	}

	return buildIngressResourceFromSpec(serviceResourceName, resource.Name, timeout, context)

}

func buildIngressHostName(resourceName voyager.ResourceName, sc wiringplugin.StateContext) string {
	formattedLabel := ""
	if string(sc.Location.Label) != "" {
		formattedLabel = fmt.Sprintf("--%s", sc.Location.Label)
	}

	envType := string(sc.Location.EnvType)

	// playground/integration have slightly different domain names
	// since they are technically "dev" but not really
	if envType == string(voyager.EnvTypeDev) {
		switch sc.ClusterConfig.KittClusterEnv {
		case kittClusterEnvIntegration:
			envType = envTypeIntegration
		case kittClusterEnvPlayground:
			envType = envTypePlayground
		}
	}

	return fmt.Sprintf("%s--%s%s.%s.%s.%s",
		sc.ServiceName,
		resourceName,
		formattedLabel,
		string(sc.Location.Region),
		envType,
		clusterHostPath)
}

func extractSingleDependencyOfType(dependencies []wiringplugin.WiredDependency, resourceType voyager.ResourceType) (*wiringplugin.WiredDependency, error) {
	var matchedDependency *wiringplugin.WiredDependency
	for x, dependency := range dependencies {
		if dependency.Type == resourceType {
			if matchedDependency != nil {
				return nil, errors.Errorf("must depend on a single %s resource, but multiple were found", resourceType)
			}
			matchedDependency = &dependencies[x]
		}
	}

	if matchedDependency == nil {
		return nil, errors.Errorf("must depend on a single %s resource, but none were found", resourceType)
	}

	return matchedDependency, nil
}

func extractKubeComputeDependencyDetails(dependencies []wiringplugin.WiredDependency) (smith_v1.ResourceName, map[string]string, error) {
	// Require exactly one KubeCompute dependency
	kubeComputeDependency, err := extractSingleDependencyOfType(dependencies, apik8scompute.ResourceType)
	if err != nil {
		return "", nil, err
	}

	shapes := kubeComputeDependency.Contract.FindAllShapes(knownshapes.LabelledShape)
	if len(shapes) == 0 {
		return "", nil, errors.New("failed to locate Kubernetes Deployment from KubeCompute dependency")
	}

	// Find labels attached to deployment object
	// TODO: Better way of identifying the correct deployment
	// Because this could break if KubeCompute ever e.g. does Blue/Green deployments
	deploymentResourceName := wiringutil.ResourceName(kubeComputeDependency.Name)
	for _, shape := range shapes {
		labelledShape, ok := shape.(*knownshapes.Labelled)
		if !ok {
			return "", nil, errors.Errorf("cannot cast LabelledShape %q to expected type", shape.Name())
		}
		if labelledShape.Data.Target == deploymentResourceName {
			return labelledShape.Data.Target, labelledShape.Data.Labels, nil
		}
	}

	return "", nil, errors.Errorf("labelled shape for deployment object %q not found", deploymentResourceName)
}
