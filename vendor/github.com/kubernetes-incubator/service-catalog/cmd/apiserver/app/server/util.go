/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-openapi/spec"
	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	admissionmetrics "k8s.io/apiserver/pkg/admission/metrics"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	apiopenapi "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericserveroptions "k8s.io/apiserver/pkg/server/options"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/kubernetes-incubator/service-catalog/pkg/api"
	scadmission "github.com/kubernetes-incubator/service-catalog/pkg/apiserver/admission"
	"github.com/kubernetes-incubator/service-catalog/pkg/apiserver/authenticator"
	"github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/internalclientset"
	informers "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/internalversion"
	"github.com/kubernetes-incubator/service-catalog/pkg/openapi"
	"github.com/kubernetes-incubator/service-catalog/pkg/util/kube"
	"github.com/kubernetes-incubator/service-catalog/pkg/version"
)

// serviceCatalogConfig is a placeholder for configuration
type serviceCatalogConfig struct {
	// the shared informers that know how to speak back to this apiserver
	sharedInformers informers.SharedInformerFactory
	// the shared informers that know how to speak back to kube apiserver
	kubeSharedInformers kubeinformers.SharedInformerFactory
	// the configured loopback client for this apiserver
	client internalclientset.Interface
	// the configured client for kube apiserver
	kubeClient kubeclientset.Interface
}

// buildGenericConfig takes the server options and produces the genericapiserver.RecommendedConfig associated with it
func buildGenericConfig(s *ServiceCatalogServerOptions) (*genericapiserver.RecommendedConfig, *serviceCatalogConfig, error) {
	// check if we are running in standalone mode (for test scenarios)
	if s.StandaloneMode {
		glog.Infof("service catalog is in standalone mode")
	}
	// server configuration options
	if err := s.SecureServingOptions.MaybeDefaultWithSelfSignedCerts(s.GenericServerRunOptions.AdvertiseAddress.String(), nil /*alternateDNS*/, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, nil, err
	}
	genericConfig := genericapiserver.NewRecommendedConfig(api.Codecs)
	if err := s.GenericServerRunOptions.ApplyTo(&genericConfig.Config); err != nil {
		return nil, nil, err
	}
	if err := s.SecureServingOptions.ApplyTo(&genericConfig.Config.SecureServing, &genericConfig.Config.LoopbackClientConfig); err != nil {
		return nil, nil, err
	}
	if !s.DisableAuth && !s.StandaloneMode {
		if err := s.AuthenticationOptions.ApplyTo(&genericConfig.Config.Authentication, genericConfig.Config.SecureServing, genericConfig.Config.OpenAPIConfig); err != nil {
			return nil, nil, err
		}
		if err := s.AuthorizationOptions.ApplyTo(&genericConfig.Config.Authorization); err != nil {
			return nil, nil, err
		}
	} else {
		// always warn when auth is disabled, since this should only be used for testing
		glog.Warning("Authentication and authorization disabled for testing purposes")
		genericConfig.Authentication.Authenticator = &authenticator.AnyUserAuthenticator{}
		genericConfig.Authorization.Authorizer = authorizerfactory.NewAlwaysAllowAuthorizer()
	}

	if err := s.AuditOptions.ApplyTo(&genericConfig.Config); err != nil {
		return nil, nil, err
	}

	if s.ServeOpenAPISpec {
		genericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
			openapi.GetOpenAPIDefinitions, apiopenapi.NewDefinitionNamer(api.Scheme))
		if genericConfig.OpenAPIConfig.Info == nil {
			genericConfig.OpenAPIConfig.Info = &spec.Info{}
		}
		if genericConfig.OpenAPIConfig.Info.Version == "" {
			if genericConfig.Version != nil {
				genericConfig.OpenAPIConfig.Info.Version = strings.Split(genericConfig.Version.String(), "-")[0]
			} else {
				genericConfig.OpenAPIConfig.Info.Version = "unversioned"
			}
		}
	} else {
		glog.Warning("OpenAPI spec will not be served")
	}

	genericConfig.SwaggerConfig = genericapiserver.DefaultSwaggerConfig()
	// TODO: investigate if we need metrics unique to service catalog, but take defaults for now
	// see https://github.com/kubernetes-incubator/service-catalog/issues/677
	genericConfig.EnableMetrics = true
	// TODO: add support to default these values in build
	// see https://github.com/kubernetes-incubator/service-catalog/issues/722
	serviceCatalogVersion := version.Get()
	genericConfig.Version = &serviceCatalogVersion

	// FUTURE: use protobuf for communication back to itself?
	client, err := internalclientset.NewForConfig(genericConfig.LoopbackClientConfig)
	if err != nil {
		glog.Errorf("Failed to create clientset for service catalog self-communication: %v", err)
		return nil, nil, err
	}
	sharedInformers := informers.NewSharedInformerFactory(client, 10*time.Minute)

	scConfig := &serviceCatalogConfig{
		client:          client,
		sharedInformers: sharedInformers,
	}
	if !s.StandaloneMode {
		clusterConfig, err := kube.LoadConfig(s.KubeconfigPath, "")
		if err != nil {
			glog.Errorf("Failed to parse kube client config: %v", err)
			return nil, nil, err
		}
		// If clusterConfig is nil, look at the default in-cluster config.
		if clusterConfig == nil {
			clusterConfig, err = restclient.InClusterConfig()
			if err != nil {
				glog.Errorf("Failed to get kube client config: %v", err)
				return nil, nil, err
			}
		}
		clusterConfig.GroupVersion = &schema.GroupVersion{}

		kubeClient, err := kubeclientset.NewForConfig(clusterConfig)
		if err != nil {
			glog.Errorf("Failed to create clientset interface: %v", err)
			return nil, nil, err
		}

		kubeSharedInformers := kubeinformers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
		genericConfig.SharedInformerFactory = kubeSharedInformers

		// TODO: we need upstream to package AlwaysAdmit, or stop defaulting to it!
		// NOTE: right now, we only run admission controllers when on kube cluster.
		genericConfig.AdmissionControl, err = buildAdmission(genericConfig, s, client, sharedInformers, kubeClient, kubeSharedInformers)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize admission: %v", err)
		}

		scConfig.kubeClient = kubeClient
		scConfig.kubeSharedInformers = kubeSharedInformers
	}

	return genericConfig, scConfig, nil
}

// buildAdmission constructs the admission chain
// TODO nilebox: Switch to RecommendedOptions and use method (a *AdmissionOptions) ApplyTo
func buildAdmission(c *genericapiserver.RecommendedConfig, s *ServiceCatalogServerOptions,
	client internalclientset.Interface, sharedInformers informers.SharedInformerFactory,
	kubeClient kubeclientset.Interface, kubeSharedInformers kubeinformers.SharedInformerFactory) (admission.Interface, error) {

	pluginNames := enabledPluginNames(s.AdmissionOptions)
	glog.Infof("Admission control plugin names: %v", pluginNames)

	genericInitializer := initializer.New(kubeClient, kubeSharedInformers, c.Authorization.Authorizer, api.Scheme)
	scPluginInitializer := scadmission.NewPluginInitializer(client, sharedInformers, kubeClient, kubeSharedInformers)
	initializersChain := admission.PluginInitializers{
		scPluginInitializer,
		genericInitializer,
	}

	pluginsConfigProvider, err := admission.ReadAdmissionConfiguration(pluginNames, s.AdmissionOptions.ConfigFile, api.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin config: %v", err)
	}
	return s.AdmissionOptions.Plugins.NewFromPlugins(pluginNames, pluginsConfigProvider, initializersChain, admission.DecoratorFunc(admissionmetrics.WithControllerMetrics))
}

// enabledPluginNames makes use of RecommendedPluginOrder, DefaultOffPlugins,
// EnablePlugins, DisablePlugins fields
// to prepare a list of ordered plugin names that are enabled.
// TODO nilebox: remove this method once switched to RecommendedOptions
func enabledPluginNames(a *genericserveroptions.AdmissionOptions) []string {
	allOffPlugins := append(a.DefaultOffPlugins.List(), a.DisablePlugins...)
	disabledPlugins := sets.NewString(allOffPlugins...)
	enabledPlugins := sets.NewString(a.EnablePlugins...)
	disabledPlugins = disabledPlugins.Difference(enabledPlugins)

	resultPlugins := sets.NewString()
	// First, add core plugins in a recommended order
	orderedPlugins := []string{}
	for _, plugin := range a.RecommendedPluginOrder {
		if !disabledPlugins.Has(plugin) {
			orderedPlugins = append(orderedPlugins, plugin)
			resultPlugins.Insert(plugin)
		}
	}
	// Second, add all missing Service Catalog plugins
	// Note that those plugins are added in no specific order
	for plugin := range enabledPlugins {
		if !resultPlugins.Has(plugin) {
			orderedPlugins = append(orderedPlugins, plugin)
			resultPlugins.Insert(plugin)
		}
	}

	return orderedPlugins
}

// addPostStartHooks adds the common post start hooks we invoke when using either server storage option.
func addPostStartHooks(server *genericapiserver.GenericAPIServer, scConfig *serviceCatalogConfig, stopCh <-chan struct{}) {
	server.AddPostStartHook("start-service-catalog-apiserver-informers", func(context genericapiserver.PostStartHookContext) error {
		glog.Infof("Starting shared informers")
		scConfig.sharedInformers.Start(stopCh)
		if scConfig.kubeSharedInformers != nil {
			scConfig.kubeSharedInformers.Start(stopCh)
		}
		glog.Infof("Started shared informers")
		return nil
	})
}
