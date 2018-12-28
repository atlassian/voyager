package luigi

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
)

type BasicServiceData struct {
	LoggingID         string `json:"logging_id,omitempty"`
	SourceID          string `json:"source_id"`
	Name              string `json:"name"`
	Organization      string `json:"organization"`
	Owner             string `json:"owner"`
	Admins            string `json:"admins,omitempty"`
	CapacityGigabytes int    `json:"capacity_gigabytes,omitempty"`
	CapacityComment   string `json:"capacity_comment,omitempty"`
}

type FullServiceData struct {
	BasicServiceData

	Acls []ServiceACL `json:"acls"`
}

type ServiceACL struct {
	Environments string `json:"environments"`
	StaffIDGroup string `json:"staffid_group"`
}

type Response struct {
	Data       json.RawMessage `json:"data"`
	StatusCode int             `json:"status_code"`
	Errors     []ErrorResponse `json:"errors"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type Client interface {
	CreateService(ctx context.Context, data *FullServiceData) (*FullServiceData, error)
	ListServices(ctx context.Context, search string) ([]BasicServiceData, error)
	DeleteService(ctx context.Context, loggingID string) error
}

// unmarshalServiceData attempts to decode the data in the response as ServiceData
func (r *Response) unmarshalServiceData(data *FullServiceData) error {
	err := json.Unmarshal(r.Data, data)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *Response) unmarshalServiceDataList(datas *[]BasicServiceData) error {
	err := json.Unmarshal(r.Data, datas)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
