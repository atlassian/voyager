package options

import (
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/options"
)

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
