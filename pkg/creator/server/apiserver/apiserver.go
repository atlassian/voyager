package apiserver

import (
	apis_creator "github.com/atlassian/voyager/pkg/apis/creator"
	"github.com/atlassian/voyager/pkg/apis/creator/install"
	creator "github.com/atlassian/voyager/pkg/creator"
	creatorrest "github.com/atlassian/voyager/pkg/creator/server/rest"
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
	ExtraConfig   *creator.ExtraConfig
}

// CreatorServer contains state for a Kubernetes cluster master/api server.
type CreatorServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *creator.ExtraConfig
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

// New returns a new instance of CreatorServer from the given config.
func (c completedConfig) New() (*CreatorServer, error) {
	genericServer, err := c.GenericConfig.New("creator-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &CreatorServer{
		GenericAPIServer: genericServer,
	}

	handler, err := creator.NewHandler(c.ExtraConfig)
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(apis_creator.GroupName, Scheme, metav1.ParameterCodec, Codecs)

	v1Storage := map[string]rest.Storage{}
	v1Storage["services"] = &creatorrest.REST{
		Logger:  c.ExtraConfig.Logger,
		Handler: handler,
	}
	apiGroupInfo.VersionedResourcesStorageMap["v1"] = v1Storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}
