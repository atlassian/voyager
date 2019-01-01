package pagerduty

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	pagerdutyClient "github.com/PagerDuty/go-pagerduty"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	namePrefix               = "Micros 2"
	escalationPolicySuffix   = "Default"
	serviceMainSuffix        = "main"
	serviceLowPrioritySuffix = "low-priority"
	nameDelimiter            = " - "

	awsInternalID = "PZQ6AUS"
)

var (
	responseRegexp = regexp.MustCompile(`HTTP response code: (\d+)`)
)

type pagerdutyRestClient interface {
	ListEscalationPolicies(o pagerdutyClient.ListEscalationPoliciesOptions) (*pagerdutyClient.ListEscalationPoliciesResponse, error)
	ListUsers(o pagerdutyClient.ListUsersOptions) (*pagerdutyClient.ListUsersResponse, error)
	ListServices(o pagerdutyClient.ListServiceOptions) (*pagerdutyClient.ListServiceResponse, error)
	CreateEscalationPolicy(e pagerdutyClient.EscalationPolicy) (*pagerdutyClient.EscalationPolicy, error)
	DeleteEscalationPolicy(id string) error
	CreateService(s pagerdutyClient.Service) (*pagerdutyClient.Service, error)
	DeleteService(id string) error
	CreateIntegration(id string, i pagerdutyClient.Integration) (*pagerdutyClient.Integration, error)
	CreateUser(u pagerdutyClient.User) (*pagerdutyClient.User, error)
}

type Client struct {
	logger        *zap.Logger
	client        pagerdutyRestClient
	uuidGenerator uuid.Generator
}

func New(logger *zap.Logger, client pagerdutyRestClient, uuidGenerator uuid.Generator) (*Client, error) {
	return &Client{
		logger:        logger,
		client:        client,
		uuidGenerator: uuidGenerator,
	}, nil
}

func GetServiceSearchURL(serviceName voyager.ServiceName) (string, error) {
	// NOTE: This URL is not used for REST API calls, it's a URL for users
	// to open in their browser to see filtered list of PagerDuty objects
	// And yes, it is using fragment (`#`) followed by a `?query=str` client-side filtering
	// also since it's a fragment we only want path escaping not query escaping and
	// (url.String() does path escaping)
	searchURL, err := url.Parse("https://atlassian.pagerduty.com/services")
	if err != nil {
		return "", err
	}
	servicePrefix := concatenateName(namePrefix, string(serviceName))
	searchURL.Fragment = "?query=" + servicePrefix
	return searchURL.String(), nil
}

func (c *Client) FindOrCreate(serviceName voyager.ServiceName, user auth.User, email string) (creator_v1.PagerDutyMetadata, error) {
	pagerdutyUser, err := c.findOrCreateUser(user, email)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}

	policy, err := c.findOrCreateEscalationPolicy(serviceName, pagerdutyUser)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}

	config, err := c.createServices(serviceName, policy)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}
	return config, err
}

func (c *Client) Delete(serviceName voyager.ServiceName) error {
	err := c.deleteService(serviceName, voyager.EnvTypeProduction, false)
	if err != nil {
		return err
	}

	err = c.deleteService(serviceName, voyager.EnvTypeProduction, true)
	if err != nil {
		return err
	}

	err = c.deleteService(serviceName, voyager.EnvTypeStaging, false)
	if err != nil {
		return err
	}

	err = c.deleteService(serviceName, voyager.EnvTypeStaging, true)
	if err != nil {
		return err
	}

	err = c.deleteEscalationPolicy(serviceName)
	if err != nil {
		return err
	}

	c.logger.Debug("completed deleting pagerduty data for this service")

	return nil
}

func (c *Client) deleteService(serviceName voyager.ServiceName, envType voyager.EnvType, lowPriority bool) error {
	pdServiceName := nameService(serviceName, envType, lowPriority)
	pdService, found, err := c.findService(pdServiceName)
	if err != nil {
		return err
	}
	if found {
		err = c.client.DeleteService(pdService.ID)
		if err != nil {
			var status int
			status, err = c.translatePagerdutyErrorToStatusCode(err)
			return clientError(status, "failed to delete service", err)
		}
		c.logger.Debug("deleted pagerduty service " + string(pdServiceName))
	}

	return nil
}

func (c *Client) deleteEscalationPolicy(serviceName voyager.ServiceName) error {
	policyName := nameEscalationPolicy(serviceName)
	policy, found, err := c.findEscalationPolicy(policyName)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	if len(policy.Services) != 0 {
		c.logger.Debug("skipping in use pagerduty escalation policy " + string(policyName))
		return nil
	}

	err = c.client.DeleteEscalationPolicy(policy.ID)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return clientError(status, "failed to delete escalation policy", err)
	}

	c.logger.Debug("deleted pagerduty escalation policy " + string(policyName))
	return nil
}

func (c *Client) findUser(email string) (*pagerdutyClient.User, bool /*found*/, error) {
	q := pagerdutyClient.ListUsersOptions{
		Query: email,
	}
	listResp, err := c.client.ListUsers(q)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return nil, false, clientError(status, "failed to list existing users", err)
	}
	for _, foundUser := range listResp.Users {
		if strings.EqualFold(foundUser.Email, email) {
			return &foundUser, true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) findOrCreateUser(user auth.User, email string) (*pagerdutyClient.User, error) {
	pdUser, found, err := c.findUser(email)
	if err != nil {
		return nil, err
	}
	if found {
		return pdUser, nil
	}

	pagerdutyUser := &pagerdutyClient.User{
		Name:  user.Name(), // NOTE: This could result in conflict because of an existing user with a different email
		Email: email,
	}

	pagerdutyUser, err = c.client.CreateUser(*pagerdutyUser)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return nil, clientError(status, "failed to create user", err)
	}
	return pagerdutyUser, nil
}

func (c *Client) findEscalationPolicy(name EscalationPolicy) (*pagerdutyClient.EscalationPolicy, bool /*found*/, error) {
	q := pagerdutyClient.ListEscalationPoliciesOptions{
		Query: string(name),
	}
	listResp, err := c.client.ListEscalationPolicies(q)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return nil, false, clientError(status, "failed to list escalation policies", err)
	}
	for _, foundEscalationPolicy := range listResp.EscalationPolicies {
		if foundEscalationPolicy.Name == string(name) {
			return &foundEscalationPolicy, true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) findOrCreateEscalationPolicy(serviceName voyager.ServiceName, user *pagerdutyClient.User) (*pagerdutyClient.EscalationPolicy, error) {
	name := nameEscalationPolicy(serviceName)

	policy, found, err := c.findEscalationPolicy(name)
	if err != nil {
		return nil, err
	}
	if found {
		return policy, nil
	}

	escalationPolicy := &pagerdutyClient.EscalationPolicy{
		Name: string(name),
		EscalationRules: []pagerdutyClient.EscalationRule{
			{
				Delay: 30, // minutes
				Targets: []pagerdutyClient.APIObject{
					{
						Type: "user",
						ID:   user.ID,
					},
				},
			},
		},
	}

	escalationPolicy, err = c.client.CreateEscalationPolicy(*escalationPolicy)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return nil, clientError(status, "failed to create escalation policy", err)
	}
	return escalationPolicy, nil
}

func (c *Client) createServices(serviceName voyager.ServiceName, policy *pagerdutyClient.EscalationPolicy) (creator_v1.PagerDutyMetadata, error) {
	var err error

	metadata := creator_v1.PagerDutyMetadata{}

	// prod
	prod := creator_v1.PagerDutyEnvMetadata{}
	prod.Main, err = c.findOrCreateService(serviceName, voyager.EnvTypeProduction, false, policy)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}
	prod.LowPriority, err = c.findOrCreateService(serviceName, voyager.EnvTypeProduction, true, policy)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}
	metadata.Production = prod

	// staging
	staging := creator_v1.PagerDutyEnvMetadata{}
	staging.Main, err = c.findOrCreateService(serviceName, voyager.EnvTypeStaging, false, policy)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}
	staging.LowPriority, err = c.findOrCreateService(serviceName, voyager.EnvTypeStaging, true, policy)
	if err != nil {
		return creator_v1.PagerDutyMetadata{}, err
	}
	metadata.Staging = staging

	return metadata, nil
}

func (c *Client) findService(pdServiceName ServiceName) (pagerdutyClient.Service, bool /* found */, error) {
	q := pagerdutyClient.ListServiceOptions{
		Query:    string(pdServiceName),
		Includes: []string{"integrations"},
	}
	listResp, err := c.client.ListServices(q)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return pagerdutyClient.Service{}, false, clientError(status, "failed to list services", err)
	}
	for _, foundService := range listResp.Services {
		if foundService.Name == string(pdServiceName) {
			return foundService, true, nil
		}
	}
	return pagerdutyClient.Service{}, false, nil
}

func (c *Client) findOrCreateService(serviceName voyager.ServiceName, envType voyager.EnvType, lowPriority bool, policy *pagerdutyClient.EscalationPolicy) (creator_v1.PagerDutyServiceMetadata, error) {
	pdServiceName := nameService(serviceName, envType, lowPriority)
	pdService, found, err := c.findService(pdServiceName)
	if err != nil {
		return creator_v1.PagerDutyServiceMetadata{}, err
	}
	if found {
		integrations, intErr := c.findOrCreateServiceIntegrations(&pdService)
		if intErr != nil {
			return creator_v1.PagerDutyServiceMetadata{}, intErr
		}
		serviceConfig := creator_v1.PagerDutyServiceMetadata{
			ServiceID:    pdService.ID,
			PolicyID:     policy.ID,
			Integrations: integrations,
		}
		return serviceConfig, nil
	}

	urgency := "high"
	if lowPriority {
		urgency = "low"
	}
	service := &pagerdutyClient.Service{
		Name: string(pdServiceName),
		EscalationPolicy: pagerdutyClient.EscalationPolicy{
			APIObject: pagerdutyClient.APIObject{
				ID:   policy.ID,
				Type: "escalation_policy_reference",
			},
		},
		IncidentUrgencyRule: &pagerdutyClient.IncidentUrgencyRule{
			Type:    "constant",
			Urgency: urgency,
		},
	}

	service, err = c.client.CreateService(*service)
	if err != nil {
		var status int
		status, err = c.translatePagerdutyErrorToStatusCode(err)
		return creator_v1.PagerDutyServiceMetadata{}, clientError(status, "failed to create service", err)
	}

	integrations, err := c.findOrCreateServiceIntegrations(service)
	if err != nil {
		return creator_v1.PagerDutyServiceMetadata{}, err
	}

	serviceConfig := creator_v1.PagerDutyServiceMetadata{
		ServiceID:    service.ID,
		PolicyID:     policy.ID,
		Integrations: integrations,
	}
	return serviceConfig, nil
}

func (c *Client) findOrCreateServiceIntegrations(service *pagerdutyClient.Service) (creator_v1.PagerDutyIntegrations, error) {
	expectedIntegrations := []*pagerdutyClient.Integration{
		// TODO: currently Micros uses "generic" integration for pollinator
		// It would be nice to eventually create a separate integration to use for that
		{Name: string(Generic), Type: "generic_events_api_inbound_integration"},
		{Name: string(Pingdom), Type: "pingdom_inbound_integration", IntegrationEmail: c.uuidGenerator.NewUUID()},
		{Name: string(CloudWatch), Type: "aws_cloudwatch_inbound_integration", Vendor: &pagerdutyClient.APIObject{
			Type: "vendor_reference",
			ID:   awsInternalID,
		}},
	}

	existingIntegrationsMap := make(map[string]*pagerdutyClient.Integration, len(service.Integrations))
	for _, integration := range service.Integrations {
		// Allocating a new variable every time, otherwise all records in the map will point to the last item in the loop :(
		integration := integration
		existingIntegrationsMap[integration.Name] = &integration
	}

	integrationsMetadata := creator_v1.PagerDutyIntegrations{}
	for _, expectedIntegration := range expectedIntegrations {
		if existingIntegration, ok := existingIntegrationsMap[expectedIntegration.Name]; ok {
			err := setIntegration(&integrationsMetadata, existingIntegration)
			if err != nil {
				return creator_v1.PagerDutyIntegrations{}, err
			}
			continue // Integration already exists, nothing to do
		}

		integration, err := c.client.CreateIntegration(service.ID, *expectedIntegration)
		if err != nil {
			var status int
			status, err = c.translatePagerdutyErrorToStatusCode(err)
			return creator_v1.PagerDutyIntegrations{}, clientError(status, "failed to create integration", err)
		}
		err = setIntegration(&integrationsMetadata, integration)
		if err != nil {
			return creator_v1.PagerDutyIntegrations{}, err
		}
	}

	return integrationsMetadata, nil
}

func (c *Client) translatePagerdutyErrorToStatusCode(err error) (int, error) {
	// So PagerDuty go library just spits back out a string
	// which is great if you're trying to figure out if there's a 404
	// This returns the status code and discards the error, or just the
	// original error.

	matches := responseRegexp.FindStringSubmatch(err.Error())
	if len(matches) <= 1 {
		return 0, err
	}
	response := matches[1]
	if len(response) == 0 {
		return 0, errors.New("unexpected response code")
	}
	i, err := strconv.ParseInt(response, 10, strconv.IntSize)
	return int(i), err
}

func setIntegration(integrations *creator_v1.PagerDutyIntegrations, integration *pagerdutyClient.Integration) error {
	integrationMetadata := creator_v1.PagerDutyIntegrationMetadata{
		IntegrationID:  integration.ID,
		IntegrationKey: integration.IntegrationKey,
	}
	switch IntegrationName(integration.Name) {
	case CloudWatch:
		integrations.CloudWatch = integrationMetadata
		return nil
	case Generic:
		integrations.Generic = integrationMetadata
		return nil
	case Pingdom:
		integrations.Pingdom = integrationMetadata
		return nil
	default:
		return errors.Errorf("Unknown integration name: %q", integration.Name)
	}
}

func nameEscalationPolicy(serviceName voyager.ServiceName) EscalationPolicy {
	return EscalationPolicy(namePagerdutyObject(string(serviceName), escalationPolicySuffix))
}

func nameService(serviceName voyager.ServiceName, envType voyager.EnvType, lowPriority bool) ServiceName {
	var suffix string
	if lowPriority {
		suffix = serviceLowPrioritySuffix
	} else {
		suffix = serviceMainSuffix
	}
	return ServiceName(namePagerdutyObject(string(serviceName), string(envType), suffix))
}

func namePagerdutyObject(objectNameParts ...string) string {
	parts := []string{namePrefix}
	parts = append(parts, objectNameParts...)
	return concatenateName(parts...)
}

func concatenateName(parts ...string) string {
	var buffer bytes.Buffer
	args := make([]interface{}, 0, len(parts))
	for i, part := range parts {
		if i > 0 {
			buffer.WriteString(nameDelimiter) // nolint
		}
		buffer.WriteString("%s") // nolint
		args = append(args, part)
	}
	return fmt.Sprintf(buffer.String(), args...)
}

// KeyToCloudWatchURL creates a URL for sending CloudWatch events to PagerDuty
func KeyToCloudWatchURL(key string) (string, error) {
	cwURL, err := url.Parse(fmt.Sprintf("https://events.pagerduty.com/adapter/cloudwatch_sns/v1/%s", key))

	if err != nil {
		return "", errors.Wrapf(err, "cannot build cloudwatch URL for key %q", key)
	}

	return cwURL.String(), nil
}

func clientError(statusCode int, message string, err error) error {
	if err == nil {
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

	return err
}
