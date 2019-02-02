package servicecentral

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	v1ServicesPath = "/api/v1/services"
	v2ServicesPath = "/api/v2/services"
	noUser         = ""

	asapAudience = "service-central"
)

// Client is a minimalistic Service Central client for our needs
// See the outdated API spec there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/docs/api.raml
// Or the actual API code (Python) there https://stash.atlassian.com/projects/MICROS/repos/service-central/browse/service_central/api.py
type Client struct {
	logger     *zap.Logger
	httpClient *http.Client
	asap       pkiutil.ASAP
	rm         *restclient.RequestMutator
}

func NewServiceCentralClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *Client {
	rm := restclient.NewRequestMutator(
		restclient.BaseURL(baseURL.String()),
	)
	return &Client{
		logger:     logger,
		httpClient: httpClient,
		asap:       asap,
		rm:         rm,
	}
}

// Create a new service. A new UUID will be allocated to this service
// return codes:
// - 201: Service created
// - 400: Bad request
// - 409: The service UUID or service name already exists
// - 500: Internal server error
func (c *Client) CreateService(ctx context.Context, user auth.User, data *ServiceData) (*ServiceData, error) {
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.Name()),
		restclient.Method(http.MethodPost),
		restclient.JoinPath(v1ServicesPath),
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

	if response.StatusCode != http.StatusCreated {
		message := fmt.Sprintf("failed to create service %q. Response: %s", data.ServiceName, respBody)
		return nil, clientError(response.StatusCode, message)
	}

	var parsedBody serviceTypeResponse
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	if len(parsedBody.Data) != 1 {
		return nil, errors.Errorf("expected single result; got %d", len(parsedBody.Data))
	}

	return &parsedBody.Data[0], nil
}

// Patch an existing service with the attached specification
// return codes:
// - 200: The service was successfully updated
func (c *Client) PatchService(ctx context.Context, user auth.User, data *ServiceData) error {
	updateData := *data
	updateData.ServiceUUID = nil
	updateData.ServiceName = ""

	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.Name()),
		restclient.Method(http.MethodPatch),
		restclient.JoinPath(fmt.Sprintf(v1ServicesPath+"/%s", url.PathEscape(*data.ServiceUUID))),
		restclient.BodyFromJSON(updateData),
		restclient.Context(ctx),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create patch service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to execute patch service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to update service %q. Response: %s", data.ServiceName, respBody)
		return clientError(response.StatusCode, message)
	}

	return nil
}

// Search for service
// - search: Search filter string, see format doc https://extranet.atlassian.com/display/MICROS/Service+Central+-+Search+Language
// - detail: Should the search query all subtables
// - limit: Limit the number of records returned
// - offset: Start returning items for a particular offset
// return codes:
// - 200: OK
// - 400: Bad request
func (c *Client) ListServices(ctx context.Context, user auth.OptionalUser, search string) ([]ServiceData, error) {
	var results []ServiceData

	offset := "0"
	for {
		response, err := c.listServices(ctx, user, search, offset)
		if err != nil {
			return nil, err
		}
		results = append(results, response.Data...)

		// service central uses a "next" url as the way to paginate through results, that url is a relative url
		// so we work around it by adding an offset param to listServices and parsing the next url to pull off
		// the offset param.... (mostly to avoid writing two versions of listServices)
		if response.Meta.Next == nil || *response.Meta.Next == "" {
			return results, nil
		}

		nextURL, err := url.Parse(*response.Meta.Next)
		if err != nil {
			return nil, errors.New("failed to parse next url when paginating results")
		}

		offset = nextURL.Query().Get("offset")
	}
}

// List recently modified services
func (c *Client) ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]ServiceData, error) {
	// The wrapper function is useless at the moment, but hopefully the endpoint will support paging functionality in the future...
	return c.listModifiedServices(ctx, user, modifiedSince)
}

func (c *Client) listServices(ctx context.Context, user auth.OptionalUser, search, offset string) (serviceTypeResponse, error) {
	mutations := []restclient.RequestMutation{
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.NameOrElse(noUser)),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(v1ServicesPath),
		restclient.Query("search", search),
		restclient.Query("expand", "tags"),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
	}

	if offset != "0" {
		mutations = append(mutations, restclient.Query("offset", url.QueryEscape(offset)))
	}

	req, err := c.rm.NewRequest(
		mutations...,
	)
	if err != nil {
		return serviceTypeResponse{}, errors.Wrap(err, "failed to create list service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return serviceTypeResponse{}, errors.Wrap(err, "failed to execute list service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return serviceTypeResponse{}, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to execute search %q. Response: %s", search, respBody)
		return serviceTypeResponse{}, clientError(response.StatusCode, message)
	}

	var parsedBody serviceTypeResponse
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return serviceTypeResponse{}, errors.Wrap(err, "failed to unmarshal response body")
	}
	return parsedBody, nil
}

func (c *Client) listModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]ServiceData, error) {
	modifiedOnStr := modifiedSince.UTC().Format(time.RFC3339)
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.NameOrElse(noUser)),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(v2ServicesPath),
		restclient.Query("modifiedOn", fmt.Sprintf(">%s", modifiedOnStr)),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create list modified services request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute list modified services request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to list modified services since %q. Response: %s", modifiedOnStr, respBody)
		return nil, clientError(response.StatusCode, message)
	}

	var parsedBody []V2Service
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	return convertV2ServicesToV1(parsedBody), nil
}

func (c *Client) GetService(ctx context.Context, user auth.OptionalUser, serviceUUID string) (*ServiceData, error) {
	l := c.logger
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.NameOrElse(noUser)),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(fmt.Sprintf(v1ServicesPath+"/%s", url.PathEscape(serviceUUID))),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create get service request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute get service request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to fetch service for %q. Response: %s", serviceUUID, respBody)
		return nil, clientError(response.StatusCode, message)
	}

	var parsedBody serviceTypeResponse
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	if len(parsedBody.Data) == 0 {
		return nil, errors.New("data is empty")
	}
	service := &parsedBody.Data[0]

	resp, err := c.GetServiceAttributes(ctx, user, serviceUUID)
	if err != nil {
		l.Error("Failed to get attributes for service", zap.Error(err), zap.String("service", serviceUUID))
	}
	ogTeamAttr, found, err := findOpsGenieTeamServiceAttribute(resp)
	if err != nil {
		// We do not return an error here as OpsGenie team is currently optional, likely to change when we remove PagerDuty
		l.Error("Failed to find OpsGenie team in service attributes", zap.Error(err))
	}

	if found {
		service.Attributes = append(service.Attributes, ogTeamAttr)
	}

	return service, nil
}

func (c *Client) DeleteService(ctx context.Context, user auth.User, serviceUUID string) error {
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.Name()),
		restclient.Method(http.MethodDelete),
		restclient.JoinPath(fmt.Sprintf(v1ServicesPath+"/%s", url.PathEscape(serviceUUID))),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
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

	if response.StatusCode != http.StatusNoContent {
		message := fmt.Sprintf("failed to delete service %q. Response: %s", serviceUUID, respBody)
		return clientError(response.StatusCode, message)
	}

	return nil
}

// GetServiceAttributes queries service central for the attributes of a given service. Can return an empty array if no attributes were found
func (c *Client) GetServiceAttributes(ctx context.Context, user auth.OptionalUser, serviceUUID string) ([]ServiceAttributeResponse, error) {
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, user.NameOrElse(noUser)),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(fmt.Sprintf(v2ServicesPath+"/%s/attributes", url.PathEscape(serviceUUID))),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create get service attributes request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute get service attributes request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to get attributes for service %q. Response: %s", serviceUUID, respBody)
		return nil, clientError(response.StatusCode, message)
	}

	var parsedBody []ServiceAttributeResponse
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	return parsedBody, nil
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

func convertV2ServicesToV1(v2Services []V2Service) []ServiceData {
	services := make([]ServiceData, 0, len(v2Services))
	for _, v2Service := range v2Services {
		services = append(services, convertV2ServiceToV1(v2Service))
	}
	return services
}

func convertV2ServiceToV1(v2Service V2Service) ServiceData {
	service := ServiceData{
		ServiceName: ServiceName(v2Service.Name),
		ServiceOwner: ServiceOwner{
			Username: v2Service.Owner,
		},
		// Tags: Missing :(
		// MiscData: Missing :(
	}
	if v2Service.UUID != "" {
		service.ServiceUUID = &v2Service.UUID
	}
	if v2Service.ServiceTier != nil {
		service.ServiceTier = *v2Service.ServiceTier
	}
	if v2Service.Platform != nil {
		service.Platform = *v2Service.Platform
	}
	if v2Service.PagerDutyServiceID != nil {
		service.PagerDutyServiceID = *v2Service.PagerDutyServiceID
	}
	if v2Service.LoggingID != nil {
		service.LoggingID = *v2Service.LoggingID
	}
	if v2Service.SSAMContainerName != nil {
		service.SSAMContainerName = *v2Service.SSAMContainerName
	}
	if v2Service.ZeroDowntimeUpgrades != nil {
		service.ZeroDowntimeUpgrades = *v2Service.ZeroDowntimeUpgrades
	}
	if v2Service.Stateless != nil {
		service.Stateless = *v2Service.Stateless
	}
	if v2Service.BusinessUnit != nil {
		service.BusinessUnit = *v2Service.BusinessUnit
	}
	creationTimestamp := v2Service.CreatedOn.UTC().Format(time.RFC3339)
	service.CreationTimestamp = &creationTimestamp
	return service
}

func findOpsGenieTeamServiceAttribute(attributes []ServiceAttributeResponse) (s ServiceAttribute, found bool, err error) {
	const opsGenieSchemaName = "opsgenie"
	for _, attr := range attributes {
		if attr.Schema.Name != opsGenieSchemaName {
			continue
		}

		team, ok := attr.Value["team"]
		if !ok {
			return ServiceAttribute{}, false, errors.Errorf("expected to find team name within schema of name %q", opsGenieSchemaName)
		}

		return ServiceAttribute{Team: team}, true, nil
	}
	return ServiceAttribute{}, false, nil
}
