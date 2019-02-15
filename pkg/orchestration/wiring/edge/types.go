package edge

import "github.com/atlassian/voyager"

// ServiceDescriptor resource spec

type Spec struct {
	UpstreamAddresses []UpstreamAddress `json:"upstreamAddresses"`
	UpstreamPort      int32             `json:"upstreamPort,omitempty"`
	UpstreamSuffix    string            `json:"upstreamSuffix,omitempty"`
	UpstreamOnly      string            `json:"upstreamOnly,omitempty"`
	Domains           []string          `json:"domains,omitempty"`
	Healthcheck       string            `json:"healthcheck,omitempty"`
	Rewrite           string            `json:"rewrite,omitempty"`
	Routes            []Route           `json:"routes,omitempty"`
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

type osbInstanceParameters struct {
	ServiceName voyager.ServiceName   `json:"serviceName"`
	Resource    osbResourceParameters `json:"resource"`
}

type osbResourceParameters struct {
	Attributes osbAttributes `json:"attributes"`
}

type osbAttributes struct {
	UpstreamAddress []osbUpstreamAddress `json:"upstream_address"`
	UpstreamPort    int32                `json:"upstream_port,omitempty"`
	UpstreamSuffix  string               `json:"upstream_suffix,omitempty"`
	UpstreamOnly    string               `json:"upstream_only,omitempty"`
	Domain          []string             `json:"domain,omitempty"`
	Healthcheck     string               `json:"healthcheck,omitempty"`
	Rewrite         string               `json:"rewrite,omitempty"`
	Routes          []osbRoute           `json:"routes,omitempty"`
}

type osbUpstreamAddress struct {
	Address string         `json:"address"`
	Region  voyager.Region `json:"region,omitempty"`
}

type osbRoute struct {
	Match    osbRouteMatch  `json:"match,omitempty"`
	Route    osbRouteAction `json:"route,omitempty"`
	Redirect string         `json:"redirect,omitempty"`
}

type osbRouteMatch struct {
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Path   string `json:"path,omitempty"`
	Host   string `json:"host,omitempty"`
}

type osbRouteAction struct {
	Cluster       string `json:"cluster"`
	PrefixRewrite string `json:"prefix_rewrite,omitempty"`
}
