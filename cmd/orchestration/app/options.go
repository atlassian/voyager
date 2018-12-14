package app

import (
	"io/ioutil"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// TagNames tell us what tag to store each of these properties under.
// e.g. ServiceName = "application_name", and the service name is put into the
// application_name tag.
type TagNames struct {
	ServiceName     voyager.Tag `json:"serviceName"`
	BusinessUnit    voyager.Tag `json:"businessUnit"`
	ResourceOwner   voyager.Tag `json:"resourceOwner"`
	Platform        voyager.Tag `json:"platform"`
	EnvironmentType voyager.Tag `json:"environmentType"`
}

type Options struct {
	Location options.Location `json:"location"`
	Cluster  options.Cluster  `json:"cluster"`
	TagNames TagNames         `json:"tagNames"`
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	allErrors = append(allErrors, o.Location.DefaultAndValidate()...)

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
