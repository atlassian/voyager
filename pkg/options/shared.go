package options

import (
	"github.com/atlassian/voyager"
	"github.com/pkg/errors"
)

type Cluster struct {
	ClusterDomainName string `json:"clusterDomainName"`
	KITTClusterEnv    string `json:"kittClusterEnv"`
	Kube2iamAccount   string `json:"kube2iamAccount"`
}

func (c *Cluster) DefaultAndValdiate() []error {
	var allErrors []error
	if c.ClusterDomainName == "" {
		allErrors = append(allErrors, errors.New("clusterDomainName must be specified in cluster"))
	}
	return allErrors
}

type Location struct {
	Account voyager.Account `json:"account"`
	Region  voyager.Region  `json:"region"`
	EnvType voyager.EnvType `json:"envType"`
}

func (l *Location) DefaultAndValidate() []error {
	var allErrors []error
	if l.Region == "" {
		allErrors = append(allErrors, errors.New("region must be specified in location"))
	}
	if l.EnvType == "" {
		allErrors = append(allErrors, errors.New("envType must be specified in location"))
	}
	if l.Account == "" {
		allErrors = append(allErrors, errors.New("account must be specified in location"))
	}
	return allErrors
}

// ClusterLocation generates a ClusterLocation based on the Location's coordinates
func (l Location) ClusterLocation() voyager.ClusterLocation {
	return voyager.ClusterLocation{
		Region:  l.Region,
		Account: l.Account,
		EnvType: l.EnvType,
	}
}
