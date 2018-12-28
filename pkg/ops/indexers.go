package ops

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
)

const (
	ServiceInstanceExternalIDIndex = "serviceInstanceExternalID"
)

// ServiceInstanceExternalIDIndexFunc indexes based on a ServiceInstance's externalID
func ServiceInstanceExternalIDIndexFunc(obj interface{}) ([]string, error) {
	serviceInstance := obj.(*sc_v1b1.ServiceInstance)
	return []string{serviceInstance.Spec.ExternalID}, nil
}
