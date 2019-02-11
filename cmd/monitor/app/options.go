package app

import (
	"io/ioutil"
	"time"

	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

type Options struct {
	ServiceDescriptorName  string                 `json:"name"`
	Location               options.Location       `json:"location"`
	ExpectedProcessingTime time.Duration          `json:"expected_processing_time"`
	ServiceSpec            creator_v1.ServiceSpec `json:"service_spec"`
}

func readAndValidateOptions(configFile string) (*Options, error) {
	doc, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var rawOptions struct {
		ServiceDescriptorName  string                 `json:"name"`
		Location               options.Location       `json:"location"`
		ExpectedProcessingTime string                 `json:"expected_processing_time"`
		ServiceSpec            creator_v1.ServiceSpec `json:"service_spec"`
	}

	err = yaml.UnmarshalStrict(doc, &rawOptions)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	processingTime, err := time.ParseDuration(rawOptions.ExpectedProcessingTime)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q, valid time units are: ns, ms, s, m, h", rawOptions.ExpectedProcessingTime)
	}

	return &Options{
		ServiceDescriptorName:  rawOptions.ServiceDescriptorName,
		Location:               rawOptions.Location,
		ExpectedProcessingTime: processingTime,
		ServiceSpec:            rawOptions.ServiceSpec,
	}, nil
}
