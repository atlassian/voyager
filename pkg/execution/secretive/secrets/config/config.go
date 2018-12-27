package config

import (
	"github.com/atlassian/voyager/pkg/execution/secretive/secrets"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateKubeClient(logger *zap.Logger, clientConfig *rest.Config, namespace string, labels map[string]string) (*secrets.Client, error) {
	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "error creating internal kube client")
	}

	return &secrets.Client{
		Logger:        logger,
		Namespace:     namespace,
		Labels:        labels,
		SecretsClient: client.CoreV1(),
	}, nil
}

func Internal() (*rest.Config, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error generating in cluster config")
	}

	return clientConfig, err
}

func External(kubeconfig string) (*rest.Config, error) {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, errors.Wrap(err, "error generating external config")
	}

	return clientConfig, err
}
