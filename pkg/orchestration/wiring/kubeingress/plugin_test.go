package kubeingress

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/stretchr/testify/assert"
	apps_v1 "k8s.io/api/apps/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func getExpectedResourceOutput(serviceResourceName smith_v1.ResourceName, resourceName voyager.ResourceName) wiringplugin.WiredSmithResource {
	return wiringplugin.WiredSmithResource{
		SmithResource: smith_v1.Resource{
			Name: wiringutil.ResourceName(resourceName),
			References: []smith_v1.Reference{
				{
					Resource: serviceResourceName,
				},
			},
			Spec: smith_v1.ResourceSpec{
				Object: &ext_v1b1.Ingress{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       k8s.IngressKind,
						APIVersion: ext_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: wiringutil.MetaName(resourceName),
						Annotations: map[string]string{
							kittIngressTypeAnnotation: "private",
							contourTimeoutAnnotation:  "60s",
						},
					},
					Spec: ext_v1b1.IngressSpec{
						Rules: []ext_v1b1.IngressRule{
							{
								Host: "--somename...k8s.atl-paas.net",
								IngressRuleValue: ext_v1b1.IngressRuleValue{
									HTTP: &ext_v1b1.HTTPIngressRuleValue{
										Paths: []ext_v1b1.HTTPIngressPath{
											{
												Path: "/",
												Backend: ext_v1b1.IngressBackend{
													ServiceName: string(serviceResourceName),
													ServicePort: intstr.FromInt(8080),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Exposed: false,
	}
}

func TestBuildingIngressResource(t *testing.T) {
	t.Parallel()

	var serviceResourceName smith_v1.ResourceName = "myResource"

	emptyStateResource := v1.StateResource{
		Name: "somename",
	}

	t.Run("E2E no ingress", func(t *testing.T) {
		var res, err = buildIngressResource(serviceResourceName, &emptyStateResource, &wiringplugin.WiringContext{})
		assert.NoError(t, err)
		assert.Equal(t, getExpectedResourceOutput(serviceResourceName, emptyStateResource.Name), res)
	})

	t.Run("from-spec no ingress", func(t *testing.T) {
		var res, err = buildIngressResourceFromSpec(serviceResourceName, emptyStateResource.Name, 60, &wiringplugin.WiringContext{})
		assert.NoError(t, err)
		assert.Equal(t, getExpectedResourceOutput(serviceResourceName, emptyStateResource.Name), res)
	})

	t.Run("from-spec timeout override", func(t *testing.T) {
		var expectedOutput = getExpectedResourceOutput(serviceResourceName, emptyStateResource.Name)
		expectedOutput.SmithResource.Spec.Object.(*ext_v1b1.Ingress).ObjectMeta.Annotations[contourTimeoutAnnotation] = "140s"
		var res, err = buildIngressResourceFromSpec(serviceResourceName, emptyStateResource.Name, 140, &wiringplugin.WiringContext{})
		assert.NoError(t, err)
		assert.Equal(t, expectedOutput, res)
	})
}

func TestExtractKubeComputeDependency(t *testing.T) {
	t.Parallel()

	deploymentObj := &apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       k8s.DeploymentKind,
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "Some Deployment",
		},
	}

	computeDep := wiringplugin.WiredDependency{
		Type: apik8scompute.ResourceType,
		SmithResources: []smith_v1.Resource{
			smith_v1.Resource{
				Spec: smith_v1.ResourceSpec{
					Object: deploymentObj,
				},
			},
		},
	}

	nonComputeDep := wiringplugin.WiredDependency{
		Type: voyager.ResourceType("MiscellaneousResource"),
		SmithResources: []smith_v1.Resource{
			smith_v1.Resource{
				Spec: smith_v1.ResourceSpec{
					Object: &apps_v1.ReplicaSet{
						TypeMeta: meta_v1.TypeMeta{
							Kind:       k8s.DeploymentKind,
							APIVersion: apps_v1.SchemeGroupVersion.String(),
						},
					},
				},
			},
		},
	}

	t.Run("valid single dependency", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{computeDep}

		res, err := extractKubeComputeDependency(deps)
		assert.NoError(t, err)
		assert.Equal(t, deploymentObj, res)
	})

	t.Run("invalid: no dependency", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{}

		_, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
	})

	t.Run("invalid: multiple dependencies", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{computeDep, computeDep}

		_, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
	})

	t.Run("valid dependency on single kubecompute and multiple non-kubecompute resource", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{nonComputeDep, computeDep, nonComputeDep}

		res, err := extractKubeComputeDependency(deps)
		assert.NoError(t, err)
		assert.Equal(t, deploymentObj, res)
	})

	t.Run("invalid: non-kubecompute dependency", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{nonComputeDep}

		_, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
	})
}

func TestBuildIngressDomainName(t *testing.T) {
	t.Parallel()
	resourceName := voyager.ResourceName("resname")
	var serviceName voyager.ServiceName = "some-service"

	testCases := []struct {
		name           string
		envType        voyager.EnvType
		region         voyager.Region
		kittClusterEnv string
		expected       string
	}{
		{
			"prod",
			voyager.EnvTypeProduction,
			"us-west-2",
			"prod",
			"some-service--resname.us-west-2.prod.k8s.atl-paas.net",
		},
		{
			"staging",
			voyager.EnvTypeStaging,
			"us-east-1",
			"staging",
			"some-service--resname.us-east-1.staging.k8s.atl-paas.net",
		},
		{
			"true dev",
			voyager.EnvTypeDev,
			"us-west-2",
			"dev1",
			"some-service--resname.us-west-2.dev.k8s.atl-paas.net",
		},
		{
			"playground dev",
			voyager.EnvTypeDev,
			"ap-southeast-2",
			"playground",
			"some-service--resname.ap-southeast-2.playground.k8s.atl-paas.net",
		},
		{
			"integration dev",
			voyager.EnvTypeDev,
			"us-west-2",
			"integration",
			"some-service--resname.us-west-2.integration.k8s.atl-paas.net",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc := wiringplugin.StateContext{
				ServiceName: serviceName,
				Location: voyager.Location{
					Region:  tc.region,
					EnvType: tc.envType,
				},
				ClusterConfig: wiringplugin.ClusterConfig{
					KittClusterEnv: tc.kittClusterEnv,
				},
			}
			actual := buildIngressHostName(resourceName, sc)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
