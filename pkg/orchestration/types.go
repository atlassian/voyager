package orchestration

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
)

// This is the 'location' label, not the kubernetes labels.
func GetNamespaceLabel(namespace *core_v1.Namespace) voyager.Label {
	label, ok := namespace.Labels[voyager.ServiceLabelLabel]

	if !ok {
		// ok if not there
		label = namespace.Annotations[voyager.ServiceLabelLabel]
	}
	return voyager.Label(label)
}

func GetNamespaceServiceName(namespace *core_v1.Namespace) (voyager.ServiceName, error) {
	serviceName, ok := namespace.Labels[voyager.ServiceNameLabel]
	if !ok {
		serviceName, ok = namespace.Annotations[voyager.ServiceNameLabel]

		if !ok {
			return "", errors.Errorf("namespace %q is missing %q label", namespace.Name, voyager.ServiceNameLabel)
		}
	}
	return voyager.ServiceName(serviceName), nil
}

// Interfaces

type Entangler interface {
	Entangle(*orch_v1.State, *EntanglerContext) (*smith_v1.Bundle, bool /*retriable*/, error)
}

// EntanglerContext contains information that is required by autowiring.
// Everything in this context can only be obtained by reading Kubernetes objects.
type EntanglerContext struct {
	// Config is the configuration pulled from the ConfigMap
	Config map[string]string

	// Label
	Label voyager.Label

	// ServiceName
	ServiceName voyager.ServiceName
}

type ClusterConfig struct {
	// ClusterDomainName is the domain name of the ingress.
	ClusterDomainName string
	KittClusterEnv    string
	Kube2iamAccount   string
}
