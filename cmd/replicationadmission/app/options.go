package app

import (
	"io/ioutil"

	"github.com/atlassian/voyager/pkg/options"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/yaml"
)

type Locations struct {
	Current    options.Location   `json:"current"`
	Replicated []options.Location `json:"replicated"`
}

type Options struct {
	ServerConfig options.ServerConfig `json:"serverConfig"`
	Locations    Locations            `json:"locations"`
}

func (locs *Locations) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, locs.Current.DefaultAndValidate()...)
	for _, l := range locs.Replicated {
		allErrors = append(allErrors, l.DefaultAndValidate()...)
	}

	if len(locs.Replicated) == 0 {
		allErrors = append(allErrors, errors.New("must have at least one replicated location (ourselves)"))
	}

	return allErrors
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, o.ServerConfig.DefaultAndValidate()...)
	allErrors = append(allErrors, o.Locations.DefaultAndValidate()...)
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
