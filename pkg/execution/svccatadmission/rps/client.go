package rps

import (
	"context"

	"github.com/atlassian/voyager"
)

type OSBResource struct {
	ServiceID  voyager.ServiceName `json:"serviceId"`
	InstanceID string              `json:"instanceId"`
}

// This is the form of errors that come back - currently we don't try to parse.
// type ErrorResponse struct {
//	 Error string `json:"error"`
// 	 Message string `json:"message"`
// }

type Client interface {
	ListOSBResources(ctx context.Context) ([]OSBResource, error)
}
