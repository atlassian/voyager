package app

import (
	"io/ioutil"

	"github.com/atlassian/voyager/pkg/options"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type Options struct {
	ServerConfig            options.ServerConfig `json:"serverConfig"`
	EnforcePRGB             bool                 `json:"enforcePRGB"`
	CompliantDockerPrefixes []string             `json:"compliantDockerPrefixes"`
}

func (o *Options) DefaultAndValidate() []error {
	return o.ServerConfig.DefaultAndValidate()
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
