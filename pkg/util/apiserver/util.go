package apiserver

import (
	"fmt"
	"io/ioutil"
	"os"
)

const (
	DefaultNamespace       = "voyager"
	inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

func GetInClusterNamespace(defaultNamespace string) (string, error) {
	// Check whether the namespace file exists.
	// If not, we are not running in cluster so can't guess the namespace.
	_, err := os.Stat(inClusterNamespacePath)
	if err != nil {
		if os.IsNotExist(err) {
			// not running in-cluster, using default namespace
			return defaultNamespace, nil
		}
		return "", fmt.Errorf("error checking namespace file: %v", err)
	}

	// Load the namespace file and return its content
	namespace, err := ioutil.ReadFile(inClusterNamespacePath)
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %v", err)
	}
	return string(namespace), nil
}
