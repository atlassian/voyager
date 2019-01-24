package app

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

type Options struct {
	Cluster string `json:"cluster"`
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

	return &opts, nil
}
