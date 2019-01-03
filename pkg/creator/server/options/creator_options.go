package options

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/pkg/creator"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
)

type CreatorOptions struct {
	LoggingOptions    options.LoggerOptions
	ConfigFileOptions *ConfigFileOptions
}

func NewCreatorOptions() *CreatorOptions {
	return &CreatorOptions{
		LoggingOptions:    options.LoggerOptions{},
		ConfigFileOptions: &ConfigFileOptions{},
	}
}

func (o *CreatorOptions) AddFlags(fs ctrl.FlagSet) {
	options.BindLoggerFlags(&o.LoggingOptions, fs)
	o.ConfigFileOptions.AddFlags(fs)
}

func (o *CreatorOptions) Validate() []error {
	var errs []error
	errs = append(errs, o.ConfigFileOptions.Validate()...)
	return errs
}

// ApplyTo adds CreatorOptions to the server configuration.
func (o *CreatorOptions) ApplyTo(config *creator.ExtraConfig) error {
	config.Logger = options.LoggerFromOptions(o.LoggingOptions)
	config.Logger.Info("Logger initialized")

	if err := o.ConfigFileOptions.ApplyTo(config); err != nil {
		return err
	}

	pd, err := pagerduty.NewPagerDutyClientConfigFromEnv()
	if err != nil {
		return err
	}
	config.PagerDuty = pd

	asapClientConfig, err := pkiutil.NewASAPClientConfigFromMicrosEnv()
	if err != nil {
		return err
	}
	config.ASAPClientConfig = asapClientConfig

	config.HTTPClient = util.HTTPClient()

	return nil
}
