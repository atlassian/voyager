package options

import (
	"net"

	"github.com/atlassian/voyager/pkg/trebuchet"
	"github.com/atlassian/voyager/pkg/trebuchet/server/apiserver"
	utilapiserver "github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeoptions "k8s.io/apiserver/pkg/server/options"
)

type TrebuchetServerOptions struct {
	RecommendedOptions *utilapiserver.RecommendedOptions
	TrebuchetOptions   *TrebuchetOptions
}

// NewTrebuchetServerOptions creates default options.
func NewTrebuchetServerOptions(processInfo *kubeoptions.ProcessInfo) *TrebuchetServerOptions {
	o := &TrebuchetServerOptions{
		RecommendedOptions: utilapiserver.NewRecommendedOptions(processInfo),
		TrebuchetOptions:   NewTrebuchetOptions(),
	}

	return o
}

// AddFlags adds flags to the flagset.
func (o *TrebuchetServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
	o.TrebuchetOptions.AddFlags(fs)
}

// Validate validates the apiserver options.
func (o *TrebuchetServerOptions) Validate() error {
	var errors []error
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in missing options.
func (o *TrebuchetServerOptions) Complete() error {
	return nil
}

// Config returns a configuration.
func (o *TrebuchetServerOptions) Config() (*apiserver.Config, error) {
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, errors.Wrap(err, "error creating self-signed certificates")
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig, apiserver.Scheme); err != nil {
		return nil, err
	}

	extraConfig := trebuchet.ExtraConfig{}
	if err := o.TrebuchetOptions.ApplyTo(&extraConfig); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   &extraConfig,
	}
	return config, nil
}
