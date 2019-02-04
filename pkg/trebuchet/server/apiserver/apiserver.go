package apiserver

import (
	apis_trebuchet "github.com/atlassian/voyager/pkg/apis/trebuchet"
	"github.com/atlassian/voyager/pkg/apis/trebuchet/install"
	trebuchetrest "github.com/atlassian/voyager/pkg/creator/server/rest"
	trebuchet "github.com/atlassian/voyager/pkg/trebuchet"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
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
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   *trebuchet.ExtraConfig
}

// TrebuchetServer contains state for a Kubernetes cluster master/api server.
type TrebuchetServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *trebuchet.ExtraConfig
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		cfg.ExtraConfig,
	}

	c.GenericConfig.EnableDiscovery = false
	c.GenericConfig.Version = &version.Info{
		Major: "0",
		Minor: "1",
	}

	return CompletedConfig{&c}
}

// New returns a new instance of TrebuchetServer from the given config.
func (c completedConfig) New() (*TrebuchetServer, error) {
	genericServer, err := c.GenericConfig.New("trebuchet-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &TrebuchetServer{
		GenericAPIServer: genericServer,
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(apis_trebuchet.GroupName, Scheme, metav1.ParameterCodec, Codecs)

	v1Storage := map[string]rest.Storage{}
	v1Storage["services"] = &trebuchetrest.REST{
		Logger: c.ExtraConfig.Logger,
	}
	apiGroupInfo.VersionedResourcesStorageMap["v1"] = v1Storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}
