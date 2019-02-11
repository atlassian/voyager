package clusterregistry

import (
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
)

type ClusterRegistry interface {
	GetClusters() ([]*cr_v1a1.Cluster, error)
}
