package wiringutil

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
)

// ServiceInstanceResourceName constructs a Smith resource name for a ServiceInstance for an Orchestration resource.
// This function is typically used to construct a resource name for the main ServiceInstance produced by an autowiring function.
func ServiceInstanceResourceName(resource voyager.ResourceName) smith_v1.ResourceName {
	return ResourceNameWithPostfix(resource, "instance")
}

// ServiceInstanceMetaName constructs a Kubernetes object meta name for a ServiceInstance for an Orchestration resource.
// This function is typically used to construct an object meta name for the main ServiceInstance produced by an autowiring function.
func ServiceInstanceMetaName(resource voyager.ResourceName) string {
	return MetaName(resource)
}
