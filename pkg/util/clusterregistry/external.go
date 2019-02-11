package clusterregistry

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	cr_v1a1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	clusters "k8s.io/cluster-registry/pkg/client/clientset/versioned/typed/clusterregistry/v1alpha1"
)

type ExternalClusterRegistry struct {
	namespace    string
	rest         *clusters.ClusterregistryV1alpha1Client
	cache        cache.ThreadSafeStore
	lastRun      time.Time
	resyncPeriod time.Duration
}

func NewExternalClusterRegistry(url string, resyncPeriod time.Duration) (*ExternalClusterRegistry, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(url, "")
	if err != nil {
		return nil, err
	}

	rest, err := clusters.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &ExternalClusterRegistry{
		namespace:    "kube-system",
		rest:         rest,
		cache:        cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
		resyncPeriod: resyncPeriod,
	}, nil

}

func (e *ExternalClusterRegistry) GetClusters() ([]*cr_v1a1.Cluster, error) {
	if time.Since(e.lastRun) > e.resyncPeriod {
		clusterList, err := e.rest.Clusters(e.namespace).List(v1.ListOptions{})

		if err != nil {
			return nil, err
		}

		clusters := map[string]interface{}{}

		for _, c := range clusterList.Items {
			if label, ok := c.Labels["customer"]; ok && label == "paas" {
				clusters[c.Name] = c.DeepCopy()
			}
		}

		e.cache.Replace(clusters, "")
		e.lastRun = time.Now()
	}

	clusters := e.cache.List()

	typedClusters := make([]*cr_v1a1.Cluster, 0)
	for _, v := range clusters {
		typedClusters = append(typedClusters, v.(*cr_v1a1.Cluster))
	}

	return typedClusters, nil
}
