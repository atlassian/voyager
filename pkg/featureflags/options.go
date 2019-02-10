package featureflags

import (
	"github.com/pkg/errors"
	"time"

	ld "gopkg.in/launchdarkly/go-client.v4"
)

type Options struct {
	LDKey string `json:"location"`
}

func (o *Options) DefaultAndValidate() []error {
	var allErrors []error
	ldClient, err := ld.MakeClient("YOUR_SDK_KEY", 5*time.Second)
	if err != nil {
		return append(allErrors, errors.Wrap(err, "failed to initialize ld client"))
	}
}

func readAndValidateOptions(configFile string) (*Options, error) {
	return &opts, nil
}
