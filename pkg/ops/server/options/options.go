package options

import (
	"net"
	"net/http"

	"github.com/atlassian/voyager/pkg/ops/server/apiserver"
	utilapiserver "github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

type OpsServerOptions struct {
	RecommendedOptions *utilapiserver.RecommendedOptions
	OpsHandler         http.Handler
}

// NewOpsServerOptions creates default options.
func NewOpsServerOptions(opsHandler http.Handler) *OpsServerOptions {
	o := &OpsServerOptions{
		RecommendedOptions: utilapiserver.NewRecommendedOptions(),
		OpsHandler:         opsHandler,
	}

	return o
}

// AddFlags adds flags to the flagset.
func (o *OpsServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
}

// Validate validates the apiserver options.
func (o *OpsServerOptions) Validate() error {
	var errs []error
	errs = append(errs, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// Complete fills in missing options.
func (o *OpsServerOptions) Complete() error {
	return nil
}

// Config returns a configuration.
func (o *OpsServerOptions) Config() (*apiserver.Config, error) {
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, errors.Wrap(err, "error creating self-signed certificates")
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)

	if err := o.RecommendedOptions.ApplyTo(serverConfig, apiserver.Scheme); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		OpsHandler:    o.OpsHandler,
	}
	return config, nil
}
