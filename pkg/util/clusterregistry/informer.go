package clusterregistry

import (
	"github.com/atlassian/ctrl"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	clientset "k8s.io/cluster-registry/pkg/client/clientset/versioned"
	informers "k8s.io/cluster-registry/pkg/client/informers/externalversions"
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

type ClusterInformer struct {
	informer cache.SharedIndexInformer
}

func NewClusterInformer(config *ctrl.Config, cctx *ctrl.Context) (*ClusterInformer, error) {
	clusterInformer := &ClusterInformer{}

	clusterGVK := cr_v1a1.SchemeGroupVersion.WithKind("clusters")
	clusterClient, err := clientset.NewForConfig(config.RestConfig)
	if err != nil {
		return nil, err
	}

	clusterInformer.informer = informers.NewSharedInformerFactory(clusterClient, config.ResyncPeriod).Clusterregistry().V1alpha1().Clusters().Informer()

	err = clusterInformer.informer.AddIndexers(cache.Indexers{
		ByClusterLabelIndexName: ByClusterLabelIndex,
	})

	if err != nil {
		return nil, err
	}

	err = cctx.RegisterInformer(clusterGVK, clusterInformer.informer)
	return clusterInformer, err
}

func (c *ClusterInformer) GetClusters() ([]*cr_v1a1.Cluster, error) {
	clusters, err := c.informer.GetIndexer().ByIndex(ByClusterLabelIndexName, "paas")
	if err != nil {
		return nil, err
	}

	typedClusters := make([]*cr_v1a1.Cluster, 0)
	for _, v := range clusters {
		typedClusters = append(typedClusters, v.(*cr_v1a1.Cluster))
	}

	return typedClusters, nil

}
