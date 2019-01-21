package app

import (
	"io/ioutil"

	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/yaml"
)

type Options struct {
	ASAPConfig pkiutil.ASAP
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, o.setupASAPConfig()...)

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

	o.ASAPConfig = asapConfig

	return []error{}
}
