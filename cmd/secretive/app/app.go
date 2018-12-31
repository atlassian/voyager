package app

import (
	"context"
	"strings"

	"github.com/atlassian/voyager/pkg/execution/secretive/asap"
	"github.com/atlassian/voyager/pkg/execution/secretive/secrets"
	"github.com/atlassian/voyager/pkg/execution/secretive/secrets/config"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type App struct {
	Logger     *zap.Logger
	Local      bool
	Kubeconfig string
	ConfigFile string
}

func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("Starting Secretive run")

	appConfig, err := ReadConfig(a.ConfigFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read config file %q", a.ConfigFile)
	}

	asapClient, err := asap.New()
	if err != nil {
		return errors.Wrap(err, "failed to create asap client")
	}

	var restConfig *rest.Config
	if a.Local {
		restConfig, err = config.External(a.Kubeconfig)
	} else {
		restConfig, err = config.Internal()
	}
	if err != nil {
		return err
	}

	secret, err := config.CreateKubeClient(a.Logger, restConfig, appConfig.Secrets.Namespace, appConfig.Secrets.Labels)
	if err != nil {
		return errors.Wrap(err, "failed to create secret client")
	}

	if errs := a.processAudiences(ctx, asapClient, secret, appConfig.Audiences); len(errs) != 0 {
		errStr := make([]string, 0, len(errs))
		for _, err := range errs {
			errStr = append(errStr, err.Error())
		}
		return errors.Errorf("error refreshing secrets:\n%s", strings.Join(errStr, "\n"))
	}
	return nil
}

func (a *App) processAudiences(ctx context.Context, asapClient *asap.Client, secretClient *secrets.Client, audiences []Audience) []error {
	a.Logger.Sugar().Infof("Processing %d audiences", len(audiences))

	var errs []error
	for _, audience := range audiences {
		if err := processAudience(ctx, asapClient, secretClient, audience); err != nil {
			errs = append(errs, errors.Wrapf(err, "processing failed for audience %s", audience))
			err = errors.Cause(err)
			if err == context.Canceled || err == context.DeadlineExceeded {
				// Return collected errors early
				return errs
			}
		}
	}
	return errs
}

func processAudience(ctx context.Context, asapClient *asap.Client, secretClient *secrets.Client, audience Audience) error {
	if audience.Lifetime == 0 {
		return errors.New("cannot generate asap token with 0 lifetime")
	}

	token, err := asapClient.Sign(audience.Name, audience.Lifetime)

	if err != nil {
		return errors.Wrap(err, "failed to generate token")
	}

	return secretClient.Write(ctx, &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      audience.Secret,
			Namespace: secretClient.Namespace,
		},
		Data: map[string][]byte{
			"token": token,
		},
		Type: core_v1.SecretTypeOpaque,
	})
}
