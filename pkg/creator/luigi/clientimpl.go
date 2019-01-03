package luigi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	servicesPath = "/api/v1/services"

	noSubject    = ""
	asapAudience = "luigi"
)

type ClientImpl struct {
	logger     *zap.Logger
	httpClient *http.Client
	rm         *restclient.RequestMutator
}

func NewLuigiClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *ClientImpl {
	rm := restclient.NewRequestMutator(
		restclient.BaseURL(baseURL.String()),
		pkiutil.AuthenticateWithASAP(asap, asapAudience, noSubject),
	)
	return &ClientImpl{
		logger:     logger,
		httpClient: httpClient,
		rm:         rm,
	}
}

func (c *ClientImpl) CreateService(ctx context.Context, data *FullServiceData) (*FullServiceData, error) {
	req, err := c.rm.NewRequest(
		restclient.Method(http.MethodPost),
		restclient.JoinPath(servicesPath),
		restclient.BodyFromJSON(data),
		restclient.Context(ctx),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create create service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute create service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	defaultMsg := fmt.Sprintf("failed to create service %q. Response: %s", data.Name, respBody)

	var serviceResponse Response
	err = json.Unmarshal(respBody, &serviceResponse)
	if err != nil {
		return nil, clientError(response.StatusCode, defaultMsg)
	}

	if response.StatusCode != http.StatusCreated {
		return nil, buildClientError(&serviceResponse, defaultMsg)
	}

	serviceData := &FullServiceData{}
	err = serviceResponse.unmarshalServiceData(serviceData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal service data")
	}

	return serviceData, nil
}

func (c *ClientImpl) ListServices(ctx context.Context, search string) ([]BasicServiceData, error) {
	req, err := c.rm.NewRequest(
		restclient.Method(http.MethodGet),
		restclient.JoinPath(servicesPath),
		restclient.Query("search", url.QueryEscape(search)),
		restclient.Header("Content-Type", "application/json"),
		restclient.Context(ctx),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create list service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute list service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	defaultMsg := fmt.Sprintf("failed to search for service %q. Response: %s", search, respBody)

	var serviceResponse Response
	err = json.Unmarshal(respBody, &serviceResponse)
	if err != nil {
		return nil, clientError(response.StatusCode, defaultMsg)
	}

	if response.StatusCode != http.StatusOK {
		return nil, buildClientError(&serviceResponse, defaultMsg)
	}

	var serviceDatas []BasicServiceData
	err = serviceResponse.unmarshalServiceDataList(&serviceDatas)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal service data")
	}
	return serviceDatas, nil
}

func (c *ClientImpl) DeleteService(ctx context.Context, loggingID string) error {
	req, err := c.rm.NewRequest(
		restclient.Method(http.MethodDelete),
		restclient.JoinPath(fmt.Sprintf(servicesPath+"/"+url.PathEscape(loggingID))),
		restclient.Context(ctx),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create delete service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to execute delete service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response body")
	}

	defaultMsg := fmt.Sprintf("failed to delete service %q. Response: %s", loggingID, respBody)

	var serviceResponse Response
	err = json.Unmarshal(respBody, &serviceResponse)
	if err != nil {
		return clientError(response.StatusCode, defaultMsg)
	}

	if response.StatusCode != http.StatusOK {
		return buildClientError(&serviceResponse, defaultMsg)
	}

	return nil
}

func buildClientError(serviceResponse *Response, defaultMsg string) error {
	errStrings := make([]string, 0, len(serviceResponse.Errors))
	for _, e := range serviceResponse.Errors {
		errStrings = append(errStrings, e.Message)
	}

	if len(errStrings) != 0 {
		return clientError(serviceResponse.StatusCode, strings.Join(errStrings, ", "))
	}
	return clientError(serviceResponse.StatusCode, defaultMsg)
}

func clientError(statusCode int, message string) error {
	switch statusCode {
	case http.StatusNotFound:
		return httputil.NewNotFound(message)
	case http.StatusBadRequest:
		return httputil.NewBadRequest(message)
	case http.StatusConflict:
		return httputil.NewConflict(message)
	default:
		return httputil.NewUnknown(fmt.Sprintf("%s (%s)", message, http.StatusText(statusCode)))
	}
}
