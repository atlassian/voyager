package apiserver

import (
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/server"
	kubeoptions "k8s.io/apiserver/pkg/server/options"
)

// NOTE: This file is mostly a copy-paste of the
// https://github.com/kubernetes/kubernetes/blob/release-1.13/staging/src/k8s.io/apiserver/pkg/server/options/recommended.go
// file, just with Etcd options removed

/*
Copyright 2016 The Kubernetes Authors.

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

// RecommendedOptions contains the recommended options for running an API server.
// If you add something to this list, it should be in a logical grouping.
// Each of them can be nil to leave the feature unconfigured on ApplyTo.
type RecommendedOptions struct {
	SecureServing  *kubeoptions.SecureServingOptionsWithLoopback
	Authentication *kubeoptions.DelegatingAuthenticationOptions
	Authorization  *kubeoptions.DelegatingAuthorizationOptions
	Audit          *kubeoptions.AuditOptions
	Features       *kubeoptions.FeatureOptions
	CoreAPI        *kubeoptions.CoreAPIOptions

	// ExtraAdmissionInitializers is called once after all ApplyTo from the options above, to pass the returned
	// admission plugin initializers to Admission.ApplyTo.
	ExtraAdmissionInitializers func(c *server.RecommendedConfig) ([]admission.PluginInitializer, error)
	Admission                  *kubeoptions.AdmissionOptions
	// ProcessInfo is used to identify events created by the server.
	ProcessInfo *kubeoptions.ProcessInfo
	Webhook     *kubeoptions.WebhookOptions
}

func NewRecommendedOptions(processInfo *kubeoptions.ProcessInfo) *RecommendedOptions {
	sso := kubeoptions.NewSecureServingOptions()

	// We are composing recommended options for an aggregated api-server,
	// whose client is typically a proxy multiplexing many operations ---
	// notably including long-running ones --- into one HTTP/2 connection
	// into this server.  So allow many concurrent operations.
	sso.HTTP2MaxStreamsPerConnection = 1000

	return &RecommendedOptions{
		SecureServing:              sso.WithLoopback(),
		Authentication:             kubeoptions.NewDelegatingAuthenticationOptions(),
		Authorization:              kubeoptions.NewDelegatingAuthorizationOptions(),
		Audit:                      kubeoptions.NewAuditOptions(),
		Features:                   kubeoptions.NewFeatureOptions(),
		CoreAPI:                    kubeoptions.NewCoreAPIOptions(),
		ExtraAdmissionInitializers: func(c *server.RecommendedConfig) ([]admission.PluginInitializer, error) { return nil, nil },
		Admission:                  kubeoptions.NewAdmissionOptions(),
		ProcessInfo:                processInfo,
		Webhook:                    kubeoptions.NewWebhookOptions(),
	}
}

func (o *RecommendedOptions) AddFlags(fs *pflag.FlagSet) {
	o.SecureServing.AddFlags(fs)
	o.Authentication.AddFlags(fs)
	o.Authorization.AddFlags(fs)
	o.Audit.AddFlags(fs)
	o.Features.AddFlags(fs)
	o.CoreAPI.AddFlags(fs)
	o.Admission.AddFlags(fs)
}

// ApplyTo adds RecommendedOptions to the server configuration.
// scheme is the scheme of the apiserver types that are sent to the admission chain.
// pluginInitializers can be empty, it is only need for additional initializers.
func (o *RecommendedOptions) ApplyTo(config *server.RecommendedConfig, scheme *runtime.Scheme) error {
	if err := o.SecureServing.ApplyTo(&config.Config.SecureServing, &config.Config.LoopbackClientConfig); err != nil {
		return err
	}
	if err := o.Authentication.ApplyTo(&config.Config.Authentication, config.SecureServing, config.OpenAPIConfig); err != nil {
		return err
	}
	if err := o.Authorization.ApplyTo(&config.Config.Authorization); err != nil {
		return err
	}
	if err := o.Audit.ApplyTo(&config.Config, config.ClientConfig, config.SharedInformerFactory, o.ProcessInfo, o.Webhook); err != nil {
		return err
	}
	if err := o.Features.ApplyTo(&config.Config); err != nil {
		return err
	}
	if err := o.CoreAPI.ApplyTo(config); err != nil {
		return err
	}
	if initializers, err := o.ExtraAdmissionInitializers(config); err != nil {
		return err
	} else if err := o.Admission.ApplyTo(&config.Config, config.SharedInformerFactory, config.ClientConfig, scheme, initializers...); err != nil {
		return err
	}

	return nil
}

func (o *RecommendedOptions) Validate() []error {
	errors := []error{}
	errors = append(errors, o.SecureServing.Validate()...)
	errors = append(errors, o.Authentication.Validate()...)
	errors = append(errors, o.Authorization.Validate()...)
	errors = append(errors, o.Audit.Validate()...)
	errors = append(errors, o.Features.Validate()...)
	errors = append(errors, o.CoreAPI.Validate()...)
	errors = append(errors, o.Admission.Validate()...)

	return errors
}
