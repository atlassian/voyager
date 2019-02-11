package options

import (
	"net"
	"net/http"

	"github.com/atlassian/voyager/pkg/aggregator/server/apiserver"
	utilapiserver "github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/request/anonymous"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeoptions "k8s.io/apiserver/pkg/server/options"
)

type AggregatorServerOptions struct {
	RecommendedOptions *utilapiserver.RecommendedOptions
	AggregatorHandler  http.Handler
	Local              bool
}

// NewAggregatorServerOptions creates default options.
func NewAggregatorServerOptions(aggregatorHandler http.Handler, processInfo *kubeoptions.ProcessInfo) *AggregatorServerOptions {
	o := &AggregatorServerOptions{
		RecommendedOptions: utilapiserver.NewRecommendedOptions(processInfo),
		AggregatorHandler:  aggregatorHandler,
	}

	return o
}

// AddFlags adds flags to the flagset.
func (o *AggregatorServerOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Local, "local", true, "Launch with local config")
	o.RecommendedOptions.AddFlags(fs)
}

// Validate validates the apiserver options.
func (o *AggregatorServerOptions) Validate() error {
	var errs []error
	errs = append(errs, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// Complete fills in missing options.
func (o *AggregatorServerOptions) Complete() error {
	return nil
}

// Config returns a configuration.
func (o *AggregatorServerOptions) Config() (*apiserver.Config, error) {
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, errors.Wrap(err, "error creating self-signed certificates")
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)

	if o.Local {
		if err := o.LocalConfig(o.RecommendedOptions, serverConfig, apiserver.Scheme); err != nil {
			return nil, err
		}
	} else {
		if err := o.RecommendedOptions.ApplyTo(serverConfig, apiserver.Scheme); err != nil {
			return nil, err
		}
	}

	config := &apiserver.Config{
		GenericConfig:     serverConfig,
		AggregatorHandler: o.AggregatorHandler,
	}
	return config, nil
}

// Setup local config, removes need to incluster configuration
func (o *AggregatorServerOptions) LocalConfig(opts *utilapiserver.RecommendedOptions, config *genericapiserver.RecommendedConfig, scheme *runtime.Scheme) error {
	if err := opts.SecureServing.ApplyTo(&config.Config.SecureServing, &config.Config.LoopbackClientConfig); err != nil {
		return err
	}
	config.Config.Authentication = genericapiserver.AuthenticationInfo{
		Authenticator: anonymous.NewAuthenticator(),
	}
	config.Config.Authorization = genericapiserver.AuthorizationInfo{
		Authorizer: authorizerfactory.NewAlwaysAllowAuthorizer(),
	}
	if err := opts.Features.ApplyTo(&config.Config); err != nil {
		return err
	}
	if err := opts.CoreAPI.ApplyTo(config); err != nil {
		return err
	}
	if initializers, err := opts.ExtraAdmissionInitializers(config); err != nil {
		return err
	} else if err := opts.Admission.ApplyTo(&config.Config, config.SharedInformerFactory, config.ClientConfig, scheme, initializers...); err != nil {
		return err
	}
	return nil
}
