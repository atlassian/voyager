package options

import (
	"io/ioutil"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/voyager/pkg/trebuchet"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/yaml"
)

type ConfigFileOptions struct {
	ConfigFile string
}

func (o *ConfigFileOptions) AddFlags(fs ctrl.FlagSet) {
	fs.StringVar(&o.ConfigFile, "config", "config.yaml", "Configuration file")
}

func (o *ConfigFileOptions) Validate() []error {
	return nil
}

// TODO: add additional configs here for trebuchet

// ApplyTo adds ConfigFileOptions to the server configuration.
func (o *ConfigFileOptions) ApplyTo(config *trebuchet.ExtraConfig) error {
	_, err := readAndValidateOptions(o.ConfigFile)
	return err
}

type parsedOptions struct {
}

type opts struct {
}

func (o *opts) defaultAndValidate() (*parsedOptions, []error) {
	return &parsedOptions{}, nil
}

func readAndValidateOptions(configFile string) (*parsedOptions, error) {
	doc, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	parseOpts := &opts{}
	if err := yaml.Unmarshal(doc, parseOpts); err != nil {
		return nil, errors.WithStack(err)
	}

	options, errs := parseOpts.defaultAndValidate()
	if len(errs) > 0 {
		return nil, utilerrors.NewAggregate(errs)
	}

	return options, nil
}
