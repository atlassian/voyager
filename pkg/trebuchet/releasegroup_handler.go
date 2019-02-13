package trebuchet

import (
	"github.com/atlassian/ctrl/options"
)

type ReleaseGroupHandler struct {
	trebuchet ReleaseInterface
}

func NewReleaseGroupHandler(config *ExtraConfig) (*ReleaseGroupHandler, error) {

	var restClientOpts options.RestClientOptions
	restConfig, err := options.LoadRestClientConfig("monitor", restClientOpts)
	if err != nil {
		return nil, err
	}

	trebuchetClient, err := trebuchet_client.NewForConfig(restConfig)

	return &ReleaseGroupHandler{
		trebuchet: trebuchetClient,
	}, nil
}
