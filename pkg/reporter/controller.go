package reporter

import (
	"github.com/atlassian/voyager"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ByServiceLabelIndexName = "serviceLabelIndex"
)

func ByServiceLabelIndex(object interface{}) ([]string, error) {
	obj := object.(meta_v1.Object)

	serviceNameLabel, ok := obj.GetLabels()[voyager.ServiceNameLabel]
	if !ok {
		return nil, nil
	}

	return []string{serviceNameLabel}, nil
}
