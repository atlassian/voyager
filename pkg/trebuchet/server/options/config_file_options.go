package options

import (
	"io/ioutil"
	"net/url"

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

// ApplyTo adds ConfigFileOptions to the server configuration.
func (o *ConfigFileOptions) ApplyTo(config *trebuchet.ExtraConfig) error {
	opts, err := readAndValidateOptions(o.ConfigFile)
	if err != nil {
		return err
	}

	config.SSAMURL = opts.SSAMURL
	config.ServiceCentralURL = opts.ServiceCentralURL

	return nil
}

type parsedOptions struct {
	SSAMURL           *url.URL
	ServiceCentralURL *url.URL
}

type opts struct {
	SSAM           string `json:"ssamURL"`
	ServiceCentral string `json:"serviceCentralURL"`
}

func (o *opts) defaultAndValidate() (*parsedOptions, []error) {
	var allErrors []error

	parsedServiceCentralURL, err := url.Parse(o.ServiceCentral)
	if err != nil {
		allErrors = append(allErrors, errors.Wrapf(err, "failed to parse Service Central URL: %q", o.ServiceCentral))
	}

	parsedSSAMURL, err := url.Parse(o.SSAM)
	if err != nil {
		allErrors = append(allErrors, errors.Wrapf(err, "failed to parse SSAM URL: %q", o.SSAM))
	}

	if len(allErrors) != 0 {
		return nil, allErrors
	}

	return &parsedOptions{
		SSAMURL:           parsedSSAMURL,
		ServiceCentralURL: parsedServiceCentralURL,
	}, nil
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
