package app

import (
	"encoding/json"
	"io/ioutil"
	"net/url"

	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/yaml"
)

type Options struct {
	ASAPClientConfig pkiutil.ASAP

	ServerConfig options.ServerConfig `json:"serverConfig"`

	Providers Providers        `json:"providers"`
	Location  options.Location `json:"location"`
}

type Providers struct {
	ServiceCentralURL *url.URL // we use custom json marshalling to read it
	RPSURL            *url.URL
	MicrosServerURL   *url.URL
}

// UnmarshalJSON unmarshals our untyped config file into a typed struct including URLs
func (p *Providers) UnmarshalJSON(data []byte) error {
	var rawProviders struct {
		ServiceCentral string `json:"serviceCentral"`
		RPSURL         string `json:"rps"`
		MicrosServer   string `json:"microsServer"`
	}

	if err := json.Unmarshal(data, &rawProviders); err != nil {
		return err
	}

	scURL, err := url.Parse(rawProviders.ServiceCentral)
	if err != nil {
		return err
	}
	rpsURL, err := url.Parse(rawProviders.RPSURL)
	if err != nil {
		return err
	}
	microsServerURL, err := url.Parse(rawProviders.MicrosServer)
	if err != nil {
		return err
	}
	p.ServiceCentralURL = scURL
	p.RPSURL = rpsURL
	p.MicrosServerURL = microsServerURL
	return nil
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, o.setupASAPConfig()...)

	if o.Providers.ServiceCentralURL == nil {
		allErrors = append(allErrors, errors.New("providers.serviceCentral must be a valid URL"))
	}

	if o.ServerConfig.TLSCert == "" {
		allErrors = append(allErrors, errors.New("Missing TLS Cert"))
	}
	if o.ServerConfig.TLSKey == "" {
		allErrors = append(allErrors, errors.New("Missing TLS Key"))
	}

	if o.ServerConfig.ServerAddr == "" {
		o.ServerConfig.ServerAddr = ":443"
	}

	return allErrors
}

func readAndValidateOptions(configFile string) (*Options, error) {
	doc, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var opts Options
	if err := yaml.Unmarshal(doc, &opts); err != nil {
		return nil, errors.WithStack(err)
	}

	errs := opts.DefaultAndValidate()
	if len(errs) > 0 {
		return nil, utilerrors.NewAggregate(errs)
	}
	return &opts, nil
}

func (o *Options) setupASAPConfig() []error {
	asapConfig, err := pkiutil.NewASAPClientConfigFromMicrosEnv()

	if err != nil {
		return []error{err}
	}

	o.ASAPClientConfig = asapConfig

	return []error{}
}
