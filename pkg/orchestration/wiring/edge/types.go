package edge

import "github.com/atlassian/voyager"

type InstanceParameters struct {
	ServiceName string             `json:"serviceName"`
	Resource    ResourceParameters `json:"resource"`
}

type ResourceParameters struct {
	Attributes Attributes `json:"attributes"`
}

type Attributes struct {
	UpstreamAddress []UpstreamAddress `json:"upstream_address"`
	UpstreamPort    int32            `json:"upstream_port,omitempty"`
	UpstreamSuffix  string           `json:"upstream_suffix,omitempty"`
	UpstreamOnly    string           `json:"upstream_only,omitempty"`
	Domain          []string         `json:"domain,omitempty"`
	Healthcheck     string           `json:"healthcheck,omitempty"`
	Rewrite         string           `json:"rewrite,omitempty"`
	Routes          Routes           `json:"routes,omitempty"`
}

 type UpstreamAddress struct {
	Address string         `json:"address"`
	Region  voyager.Region `json:"region,omitempty"`
}

type Routes []struct {
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
	PrefixRewrite string `json:"prefix_rewrite,omitempty"`
}
