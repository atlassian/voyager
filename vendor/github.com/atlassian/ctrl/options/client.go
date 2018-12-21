package options

import (
	"github.com/atlassian/ctrl"
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	DefaultAPIQPS     = 5
	APIQPSBurstFactor = 1.5
)

type RestClientOptions struct {
	APIQPS               float64
	ClientConfigFileFrom string
	ClientConfigFileName string
	ClientContext        string
}

func (o *RestClientOptions) DefaultAndValidate() []error {
	var allErrors []error
	if o.APIQPS == 0 {
		o.APIQPS = DefaultAPIQPS
	}
	if o.APIQPS < 0 {
		allErrors = append(allErrors, errors.Errorf("value for API QPS must be non-negative. Given: %f", o.APIQPS))
	}
	return allErrors
}

func BindRestClientFlags(o *RestClientOptions, fs ctrl.FlagSet) {
	fs.Float64Var(&o.APIQPS, "api-qps", DefaultAPIQPS, "Maximum queries per second when talking to Kubernetes API")
	fs.StringVar(&o.ClientConfigFileFrom, "client-config-from", "in-cluster",
		"Source of REST client configuration. 'in-cluster' (default) and 'file' are valid options.")
	fs.StringVar(&o.ClientConfigFileName, "client-config-file-name", "",
		"Load REST client configuration from the specified Kubernetes config file. This is only applicable if --client-config-from=file is set.")
	fs.StringVar(&o.ClientContext, "client-config-context", "",
		"Context to use for REST client configuration. This is only applicable if --client-config-from=file is set.")
}

func LoadRestClientConfig(userAgent string, options RestClientOptions) (*rest.Config, error) {
	var config *rest.Config
	var err error

	switch options.ClientConfigFileFrom {
	case "in-cluster":
		config, err = rest.InClusterConfig()
	case "file":
		var configAPI *clientcmdapi.Config
		configAPI, err = clientcmd.LoadFromFile(options.ClientConfigFileName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load REST client configuration from file %q", options.ClientConfigFileName)
		}
		config, err = clientcmd.NewDefaultClientConfig(*configAPI, &clientcmd.ConfigOverrides{
			CurrentContext: options.ClientContext,
		}).ClientConfig()
	default:
		err = errors.New("invalid value for 'client config from' parameter")
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load REST client configuration from %q", options.ClientConfigFileFrom)
	}
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(float32(options.APIQPS), int(options.APIQPS*APIQPSBurstFactor))
	config.UserAgent = userAgent
	return config, nil
}
