package options

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/pkg/trebuchet"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
)

type TrebuchetOptions struct {
	LoggingOptions    options.LoggerOptions
	ConfigFileOptions *ConfigFileOptions
}

func NewTrebuchetOptions() *TrebuchetOptions {
	return &TrebuchetOptions{
		LoggingOptions:    options.LoggerOptions{},
		ConfigFileOptions: &ConfigFileOptions{},
	}
}

func (o *TrebuchetOptions) AddFlags(fs ctrl.FlagSet) {
	options.BindLoggerFlags(&o.LoggingOptions, fs)
	o.ConfigFileOptions.AddFlags(fs)
}

func (o *TrebuchetOptions) Validate() []error {
	var errs []error
	errs = append(errs, o.ConfigFileOptions.Validate()...)
	return errs
}

// ApplyTo adds CreatorOptions to the server configuration.
func (o *TrebuchetOptions) ApplyTo(config *trebuchet.ExtraConfig) error {
	config.Logger = options.LoggerFromOptions(o.LoggingOptions)
	config.Logger.Info("Logger initialized")

	if err := o.ConfigFileOptions.ApplyTo(config); err != nil {
		return err
	}

	asapClientConfig, err := pkiutil.NewASAPClientConfigFromMicrosEnv()
	if err != nil {
		return err
	}
	config.ASAPClientConfig = asapClientConfig

	config.HTTPClient = util.HTTPClient()

	return nil
}
