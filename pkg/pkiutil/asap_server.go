package pkiutil

import (
	"flag"
	"strings"

	"bitbucket.org/atlassian/go-asap/keyprovider"
	"go.uber.org/zap"
)

type ASAPServerOptions struct {
	Defaults                 ASAPServerOptionsDefaults
	asapEnabled              bool
	audience                 string
	publicKeyRepoURL         string
	fallbackPublicKeyRepoURL string
	clients                  string
}

type ASAPServerOptionsDefaults struct {
	ASAPEnabled  bool
	Audience     string
	PublicKeyURL string
	Clients      string
}

type ASAPServerConfig struct {
	ASAPEnabled       bool
	Audience          string
	PublicKeyProvider keyprovider.PublicKeyProvider
	Clients           []string
}

func (o *ASAPServerOptions) AddFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.asapEnabled, "asap_enabled", o.Defaults.ASAPEnabled, "Enable ASAP")
	fs.StringVar(&o.audience, "asap_audience", o.Defaults.Audience, "Incoming requests need to have this audience")
	fs.StringVar(&o.clients, "asap_clients", o.Defaults.Clients, "Comma separated list of authorised clients")
	fs.StringVar(&o.publicKeyRepoURL, "asap_public_key_repository_url", o.Defaults.PublicKeyURL, "ASAP repo URL")
	fs.StringVar(&o.fallbackPublicKeyRepoURL, "asap_public_key_fallback_repository_url", o.Defaults.PublicKeyURL, "ASAP fallback repo URL")
}

func (o *ASAPServerOptions) CreateConfig(logger *zap.Logger) ASAPServerConfig {
	return ASAPServerConfig{
		ASAPEnabled: o.asapEnabled,
		Audience:    o.audience,
		Clients:     strings.Split(o.clients, ","),
		PublicKeyProvider: NewMirroredPublicKeyProvider(
			logger,
			&keyprovider.HTTPPublicKeyProvider{
				BaseURL: o.publicKeyRepoURL,
			},
			&keyprovider.HTTPPublicKeyProvider{
				BaseURL: o.fallbackPublicKeyRepoURL,
			},
		),
	}
}
