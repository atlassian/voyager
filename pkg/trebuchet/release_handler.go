package trebuchet

import (
	"github.com/atlassian/ctrl/options"
)

type ReleaseHandler struct {
	trebuchet ReleaseInterface
}

func NewReleaseHandler(config *ExtraConfig) (*ReleaseHandler, error) {

	var restClientOpts options.RestClientOptions
	restConfig, err := options.LoadRestClientConfig("monitor", restClientOpts)
	if err != nil {
		return nil, err
	}

	trebuchetClient, err := trebuchet_client.NewForConfig(restConfig)

	return &ReleaseHandler{
		trebuchet: trebuchetClient,
	}, nil
}
