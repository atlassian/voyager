package options

import (
	"io"
	"net"

	"github.com/atlassian/voyager/pkg/creator"
	"github.com/atlassian/voyager/pkg/creator/server/apiserver"
	utilapiserver "github.com/atlassian/voyager/pkg/util/apiserver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

type CreatorServerOptions struct {
	RecommendedOptions *utilapiserver.RecommendedOptions
	CreatorOptions     *CreatorOptions

	StdOut io.Writer
	StdErr io.Writer
}

// NewCreatorServerOptions creates default options.
func NewCreatorServerOptions(out, errOut io.Writer) *CreatorServerOptions {
	o := &CreatorServerOptions{
		RecommendedOptions: utilapiserver.NewRecommendedOptions(),
		CreatorOptions:     NewCreatorOptions(),

		StdOut: out,
		StdErr: errOut,
	}

	return o
}

// AddFlags adds flags to the flagset.
func (o *CreatorServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
	o.CreatorOptions.AddFlags(fs)
}

// Validate validates the apiserver options.
func (o *CreatorServerOptions) Validate() error {
	var errors []error
	errors = append(errors, o.RecommendedOptions.Validate()...)
	errors = append(errors, o.CreatorOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in missing options.
func (o *CreatorServerOptions) Complete() error {
	return nil
}

// Config returns a configuration.
func (o *CreatorServerOptions) Config() (*apiserver.Config, error) {
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, errors.Wrap(err, "error creating self-signed certificates")
	}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig, apiserver.Scheme); err != nil {
		return nil, err
	}

	extraConfig := creator.ExtraConfig{}
	if err := o.CreatorOptions.ApplyTo(&extraConfig); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   &extraConfig,
	}
	return config, nil
}
