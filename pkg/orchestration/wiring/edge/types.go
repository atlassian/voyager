package edge

import "github.com/atlassian/voyager"

// ServiceDescriptor resource spec

type Spec struct {
	UpstreamAddresses []UpstreamAddress `json:"upstreamAddresses"`
	UpstreamPort    int32                `json:"upstreamPort,omitempty"`
	UpstreamSuffix  string               `json:"upstreamSuffix,omitempty"`
	UpstreamOnly    string               `json:"upstreamOnly,omitempty"`
	Domains          []string             `json:"domains,omitempty"`
	Healthcheck     string               `json:"healthcheck,omitempty"`
	Rewrite         string               `json:"rewrite,omitempty"`
	Routes          []Route              `json:"routes,omitempty"`
}

type UpstreamAddress struct {
	Address string         `json:"address"`
	Region  voyager.Region `json:"region,omitempty"`
}

type Route struct {
	Match    RouteMatch  `json:"match,omitempty"`
	Route    RouteAction `json:"route,omitempty"`
	Redirect string      `json:"redirect,omitempty"`
}

type RouteMatch struct {
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Path   string `json:"path,omitempty"`
	Host   string `json:"host,omitempty"`
}

type RouteAction struct {
	Cluster       string `json:"cluster"`
	PrefixRewrite string `json:"prefixRewrite,omitempty"`
}

// OSB parameters

type OSBInstanceParameters struct {
	ServiceName voyager.ServiceName   `json:"serviceName"`
	Resource    OSBResourceParameters `json:"resource"`
}

type OSBResourceParameters struct {
	Attributes OSBAttributes `json:"attributes"`
}

type OSBAttributes struct {
	UpstreamAddress []OSBUpstreamAddress `json:"upstream_address"`
	UpstreamPort    int32                `json:"upstream_port,omitempty"`
	UpstreamSuffix  string               `json:"upstream_suffix,omitempty"`
	UpstreamOnly    string               `json:"upstream_only,omitempty"`
	Domain          []string             `json:"domain,omitempty"`
	Healthcheck     string               `json:"healthcheck,omitempty"`
	Rewrite         string               `json:"rewrite,omitempty"`
	Routes          []OSBRoute           `json:"routes,omitempty"`
}

type OSBUpstreamAddress struct {
	Address string         `json:"address"`
	Region  voyager.Region `json:"region,omitempty"`
}

type OSBRoute struct {
	Match    OSBRouteMatch  `json:"match,omitempty"`
	Route    OSBRouteAction `json:"route,omitempty"`
	Redirect string         `json:"redirect,omitempty"`
}

type OSBRouteMatch struct {
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Path   string `json:"path,omitempty"`
	Host   string `json:"host,omitempty"`
}

type OSBRouteAction struct {
	Cluster       string `json:"cluster"`
	PrefixRewrite string `json:"prefix_rewrite,omitempty"`
}
