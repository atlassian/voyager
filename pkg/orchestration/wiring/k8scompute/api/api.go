package apik8scompute

import "github.com/atlassian/voyager"

const (
	ResourceType voyager.ResourceType = "KubeCompute"

	// hard coded this secret to be able to pull images from docker-atl-paas
	// we will revisit this later for more generic approach
	DockerImagePullName = "kubecompute-docker-atl-paas"

	// CommonSecretName is the name of a secret all computes for a particular
	// namespace will have in envFrom
	CommonSecretName = "common-secrets"
)
