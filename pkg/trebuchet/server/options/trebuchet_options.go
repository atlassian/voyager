package options

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/pkg/trebuchet"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
)

// TODO: Add logging options for trebuchet
type TrebuchetOptions struct {
	LoggingOptions options.LoggerOptions
}

func NewTrebuchetOptions() *TrebuchetOptions {
	return &TrebuchetOptions{
		LoggingOptions: options.LoggerOptions{},
	}
}

func (o *TrebuchetOptions) AddFlags(fs ctrl.FlagSet) {
	options.BindLoggerFlags(&o.LoggingOptions, fs)
}

// ApplyTo adds CreatorOptions to the server configuration.
func (o *TrebuchetOptions) ApplyTo(config *trebuchet.ExtraConfig) error {
	config.Logger = options.LoggerFromOptions(o.LoggingOptions)
	config.Logger.Info("Logger initialized")

	asapClientConfig, err := pkiutil.NewASAPClientConfigFromMicrosEnv()
	if err != nil {
		return err
	}
	config.ASAPClientConfig = asapClientConfig

	config.HTTPClient = util.HTTPClient()

	return nil
}
