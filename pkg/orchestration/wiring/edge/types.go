package edge

import "github.com/atlassian/voyager"

type Parameters struct {
	UpstreamAddress `json:"upstream_address"`
	UpstreamPort    int      `json:"upstream_port,omitempty"`
	UpstreamSuffix  string   `json:"upstream_suffix,omitempty"`
	UpstreamOnly    string   `json:"upstream_only,omitempty"`
	Domain          []string `json:"domain,omitempty"`
	Healthcheck     string   `json:"healthcheck,omitempty"`
	Rewrite         string   `json:"rewrite,omitempty"`
	Routes          `json:"routes"`
}

type UpstreamAddress []struct {
	Address string         `json:"address"`
	Region  voyager.Region `json:"region,omitempty"`
}

type Routes []struct {
	RouteMatch     `json:"match,omitempty"`
	RouteAction    `json:"route,omitempty"`
	RedirectAction string `json:"redirect,omitempty"`
}

type RouteMatch struct {
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Path   string `json:"path,omitempty"`
}

type RouteAction struct {
	Cluster       string `json:"cluster"`
	PrefixRewrite string `json:"prefix_rewrite,omitempty"`
}
