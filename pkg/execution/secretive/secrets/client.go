package secrets

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core_v1_client "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Client struct {
	Logger        *zap.Logger
	Namespace     string
	Labels        labels.Set
	SecretsClient core_v1_client.SecretsGetter
}

func (client *Client) Write(ctx context.Context, secret *core_v1.Secret) error {
	nameSpaced := client.SecretsClient.Secrets(client.Namespace)
	retry := false
	for {
		if retry {
			select {
			case <-ctx.Done():
				client.Logger.Sugar().Infof("Terminated early working on secret: %s", secret.Name)
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
		prev, err := nameSpaced.Get(secret.Name, metav1.GetOptions{})

		if err != nil {
			if kubeErrors.IsNotFound(err) {
				_, createErr := nameSpaced.Create(secret)
				if createErr != nil {
					if kubeErrors.IsAlreadyExists(createErr) {
						retry = true
						continue // Concurrent create, retry
					}
					return errors.Wrapf(createErr, "unable to create secret: %s", secret.Name)
				}
				client.Logger.Sugar().Infof("Created secret: %s", secret.Name)
				break
			}
			return errors.Wrapf(err, "unexpected error condition for get secret: %s", secret.Name)
		}
		prev.Data = secret.Data

		_, err = nameSpaced.Update(prev)
		if err == nil {
			client.Logger.Sugar().Infof("Updated secret: %s", secret.Name)
			break
		}
		if kubeErrors.IsConflict(err) || kubeErrors.IsNotFound(err) {
			retry = true
			continue // Concurrent update/delete, retry
		}
		return errors.Wrapf(err, "failed to update secret: %s", secret.Name)
	}

	return nil
}
