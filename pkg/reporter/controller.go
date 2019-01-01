package reporter

import (
	"github.com/atlassian/voyager/pkg/util/layers"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ByServiceNameLabelIndexName = "serviceLabelIndex"
)

func ByServiceNameLabelIndex(object interface{}) ([]string, error) {
	nsObj := object.(meta_v1.Object)

	serviceNameLabel, err := layers.ServiceNameFromNamespaceLabels(nsObj.GetLabels())
	if err != nil {
		return nil, nil
	}

	return []string{string(serviceNameLabel)}, nil
}
