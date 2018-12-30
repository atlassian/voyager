package aggregator

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ByClusterLabelIndexName = "clusterLabelIndex"
	CustomerLabel           = "customer"
)

func ByClusterLabelIndex(object interface{}) ([]string, error) {
	obj := object.(meta_v1.Object)

	clusterLabel, ok := obj.GetLabels()[CustomerLabel]
	if !ok {
		return nil, nil
	}

	return []string{clusterLabel}, nil
}
