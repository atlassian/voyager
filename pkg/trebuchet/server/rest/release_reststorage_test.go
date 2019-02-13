package rest

import "k8s.io/apiserver/pkg/registry/rest"

var (
	_ rest.Storage         = &REST{}
	_ rest.Scoper          = &REST{}
	_ rest.Lister          = &REST{}
	_ rest.Getter          = &REST{}
	_ rest.CreaterUpdater  = &REST{}
	_ rest.GracefulDeleter = &REST{}
)
