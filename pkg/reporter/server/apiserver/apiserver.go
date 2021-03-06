package apiserver

import (
	"net/http"

	"github.com/atlassian/voyager/pkg/apis/reporter/install"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	install.Install(Scheme)

	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

type Config struct {
	GenericConfig   *genericapiserver.RecommendedConfig
	ReporterHandler http.Handler
}

// ReporterServer contains state for a Kubernetes cluster master/api server.
type ReporterServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig   genericapiserver.CompletedConfig
	ReporterHandler http.Handler
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		cfg.ReporterHandler,
	}

	c.GenericConfig.EnableDiscovery = false
	c.GenericConfig.Version = &version.Info{
		Major: "0",
		Minor: "1",
	}

	return CompletedConfig{&c}
}

// New returns a new instance of ReporterServer from the given config.
func (c completedConfig) New() (*ReporterServer, error) {
	delegationTarget := genericapiserver.NewEmptyDelegate()
	genericServer, err := c.GenericConfig.New("reporter-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &ReporterServer{
		GenericAPIServer: genericServer,
	}

	// Default API group conflicts with our handler so we just passthrough everything
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle("/apis", c.ReporterHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.HandlePrefix("/apis/", c.ReporterHandler)

	return s, nil
}
