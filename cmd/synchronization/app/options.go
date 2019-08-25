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

	Providers           Providers        `json:"providers"`
	Location            options.Location `json:"location"`
	AllowMutateServices bool             `json:"allowMutateServices"`
}

type Providers struct {
	ServiceCentralURL              *url.URL // we use custom json marshalling to read it
	DeployinatorURL                *url.URL
	OpsgenieIntegrationsManagerURL *url.URL
}

// UnmarshalJSON unmarshals our untyped config file into a typed struct including URLs
func (p *Providers) UnmarshalJSON(data []byte) error {
	var rawProviders struct {
		ServiceCentral              string `json:"serviceCentral"`
		Deployinator                string `json:"deployinator"`
		OpsgenieIntegrationsManager string `json:"opsgenieIntegrationsManager"`
	}

	if err := json.Unmarshal(data, &rawProviders); err != nil {
		return err
	}

	scURL, err := url.Parse(rawProviders.ServiceCentral)
	if err != nil {
		return errors.Wrap(err, "unable to parse Service Central URL")
	}
	p.ServiceCentralURL = scURL

	depURL, err := url.Parse(rawProviders.Deployinator)
	if err != nil {
		return errors.Wrap(err, "unable to parse Deployinator URL")
	}
	p.DeployinatorURL = depURL

	ogUrl, err := url.Parse(rawProviders.OpsgenieIntegrationsManager)
	if err != nil {
		return errors.Wrap(err, "unable to parse Opsgenie Integrations Manager URL")
	}
	p.OpsgenieIntegrationsManagerURL = ogUrl

	return nil
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, o.setupASAPConfig()...)

	if o.Providers.ServiceCentralURL == nil {
		allErrors = append(allErrors, errors.New("providers.serviceCentral must be a valid URL"))
	}

	if o.Providers.OpsgenieIntegrationsManagerURL == nil {
		allErrors = append(allErrors, errors.New("providers.OpsgenieIntegrationsManagerURL must be a valid URL"))
	}

	return allErrors
}

func readAndValidateOptions(configFile string) (*Options, error) {
	doc, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var opts Options
	if err := yaml.UnmarshalStrict(doc, &opts); err != nil {
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
