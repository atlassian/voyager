package kubeingress

import (
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
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

	// The port is hardcoded for now, see https://sdog.jira-dev.com/browse/MICROS-6451
	servicePort     = 8080
	clusterHostPath = "k8s.atl-paas.net"

	servicePostfix = "service"
	ingressPostfix = "ingress"
)

// WireUp is the main autowiring function for KubeIngress
func WireUp(resource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool /*retriable*/, error) {

	// Fail if the resource type is wrong
	if resource.Type != ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", resource.Type)
	}

	deploymentSpec, retriable, err := extractKubeComputeDependency(context.Dependencies)
	if err != nil {
		return nil, retriable, err
	}

	deploymentName := deploymentSpec.Name
	deploymentLabels := deploymentSpec.Spec.Template.ObjectMeta.Labels

	var smithResources []wiringplugin.WiredSmithResource

	// Build the Service
	serviceResource := buildServiceResource(smith_v1.ResourceName(deploymentName), deploymentLabels, resource, context)
	smithResources = append(smithResources, serviceResource)

	// Build the Ingress
	ingressResource := buildIngressResource(serviceResource.SmithResource.Name, resource, context)
	smithResources = append(smithResources, ingressResource)

	result := &wiringplugin.WiringResult{
		Resources: smithResources,
	}

	return result, false, nil
}

// buildServiceResource constructs the Kube Service object
func buildServiceResource(deploymentName smith_v1.ResourceName, selectorLabels map[string]string, resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiredSmithResource {
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

	serviceResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
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
		},
		Exposed: false,
	}

	return serviceResource
}

// buildIngressResource constructs the Kube / KITT Ingress object
func buildIngressResource(serviceName smith_v1.ResourceName, resource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiredSmithResource {
	ingressName := string(resource.Name) + "-" + ingressPostfix
	hostname := buildIngressHostName(resource.Name, context.StateContext)

	ingressRule := ext_v1b1.IngressRule{
		Host: hostname,
		IngressRuleValue: ext_v1b1.IngressRuleValue{
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
		},
	}

	ingressSpec := ext_v1b1.IngressSpec{
		Rules: []ext_v1b1.IngressRule{
			ingressRule,
		},
	}

	ingressResource := wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
			Name: smith_v1.ResourceName(ingressName),
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
						Name: ingressName,
						Annotations: map[string]string{
							kittIngressTypeAnnotation: "private",
						},
					},
					Spec: ingressSpec,
				},
			},
		},
		Exposed: true,
	}

	return ingressResource
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

func extractKubeComputeDependency(dependencies []wiringplugin.WiredDependency) (*apps_v1.Deployment, bool /* retriable */, error) {
	// Require exactly one KubeCompute dependency
	if len(dependencies) != 1 || dependencies[0].Type != k8scompute.ResourceType {
		return nil, false, errors.Errorf("must depend on a single %s resource. %d dependencies were given", k8scompute.ResourceType, len(dependencies))
	}

	// Extract the deployment created by the KubeCompute dependency
	var deploymentResource *smith_v1.Resource
	for _, res := range dependencies[0].SmithResources {
		// TODO: Better way of identifying the correct deployment
		// Because this could break if KubeCompute ever e.g. does Blue/Green deployments
		if res.Spec.Object.GetObjectKind().GroupVersionKind() == k8s.DeploymentGVK {
			deploymentResource = &res
			break
		}
	}

	if deploymentResource == nil {
		return nil, false, errors.New("failed to locate Kubernetes Deployment from KubeCompute dependency")
	}

	deploymentSpec, ok := deploymentResource.Spec.Object.(*apps_v1.Deployment)
	if !ok {
		return nil, false, errors.New("cannot cast Deployment to expected spec type")
	}

	return deploymentSpec, false, nil
}
