package kubeingress

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/k8scompute/api"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/stretchr/testify/assert"
	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

		res, _, err := extractKubeComputeDependency(deps)
		assert.NoError(t, err)
		assert.ObjectsAreEqual(deploymentObj, res)
	})

	t.Run("invalid: no dependency", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{}

		_, retriable, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
		assert.False(t, retriable)
	})

	t.Run("invalid: multiple dependencies", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{computeDep, computeDep}

		_, retriable, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
		assert.False(t, retriable)
	})

	t.Run("invalid: non-kubecompute dependency", func(t *testing.T) {
		deps := []wiringplugin.WiredDependency{nonComputeDep}

		_, retriable, err := extractKubeComputeDependency(deps)
		assert.Error(t, err)
		assert.False(t, retriable)
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
