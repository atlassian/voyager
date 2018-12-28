package composition

import (
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/voyager"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FinalizerServiceDescriptorComposition = voyager.Domain + "/serviceDescriptorComposition"
)

func hasServiceDescriptorFinalizer(accessor meta_v1.Object) bool {
	return resources.HasFinalizer(accessor, FinalizerServiceDescriptorComposition)
}

func addServiceDescriptorFinalizer(finalizers []string) []string {
	return append(finalizers, FinalizerServiceDescriptorComposition)
}

func removeServiceDescriptorFinalizer(finalizers []string) []string {
	newFinalizers := make([]string, 0, len(finalizers))
	for _, finalizer := range finalizers {
		if finalizer == FinalizerServiceDescriptorComposition {
			continue
		}
		newFinalizers = append(newFinalizers, finalizer)
	}
	return newFinalizers
}
