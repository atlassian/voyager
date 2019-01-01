package layers

import (
	"github.com/atlassian/voyager"
	"github.com/pkg/errors"
)

// ServiceLabelFromNamespaceLabels extracts a Service label from Namespace's labels.
// Service's Label can be attached as a Kubernetes label to a Namespace that corresponds to a Service Name + Label location.
func ServiceLabelFromNamespaceLabels(namespaceLabels map[string]string) voyager.Label {
	return voyager.Label(namespaceLabels[voyager.ServiceLabelLabel])
}

// ServiceNameFromNamespaceLabels extracts a Service Name from Namespace's labels.
// Service's name can be attached as a Kubernetes label to a Namespace that corresponds to a Service Name + Label location.
func ServiceNameFromNamespaceLabels(namespaceLabels map[string]string) (voyager.ServiceName, error) {
	serviceName, ok := namespaceLabels[voyager.ServiceNameLabel]
	if !ok {
		return "", errors.Errorf("namespace is missing %q label", voyager.ServiceNameLabel)
	}
	if serviceName == "" {
		return "", errors.Errorf("label %q has empty value", voyager.ServiceNameLabel)
	}
	return voyager.ServiceName(serviceName), nil
}
