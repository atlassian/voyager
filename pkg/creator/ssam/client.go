package ssam

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/creator/ssam/util/zappers"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/atlassian/voyager/pkg/util/validation"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	baseURL      = "/api/access"
	asapAudience = "ssam"
	asapSubject  = ""
)

func urlCreateContainer() string {
	return fmt.Sprintf("%s/containers/", baseURL)
}

func urlGetContainer(containerName string) string {
	return fmt.Sprintf("%s/containers/%s/", baseURL, url.PathEscape(containerName))
}

func urlCreateContainerAccessLevel(containerName string) string {
	return fmt.Sprintf("%s/containers/%s/access-levels/", baseURL, url.PathEscape(containerName))
}

func urlGetContainerAccessLevel(containerName string, accessLevelName string) string {
	return fmt.Sprintf("%s/containers/%s/access-levels/%s/", baseURL, url.PathEscape(containerName), url.PathEscape(accessLevelName))
}

func (c *clientImpl) getErrorFromResponse(resp *http.Response, logger *zap.Logger) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		logger.Info("Authorization has failed with ASAP key",
			zappers.ASAPKeyID(c.asap.KeyID()),
			zappers.ASAPKeyIssuer(c.asap.KeyIssuer()))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "unable to read response body (statusCode=%d)", resp.StatusCode)
	}
	return errors.Errorf("statusCode=%d, body=%q", resp.StatusCode, string(body))
}

// TODO validate that the SystemOwner, DisplayName, ShortName, and ContainerType have all been set
type ContainerPostRequest struct {
	ContainerType string   `json:"container_type" validate:"required"`
	DisplayName   string   `json:"display_name" validate:"required"`
	ShortName     string   `json:"short_name" validate:"required"`
	URL           string   `json:"url,omitempty"`
	SystemOwner   string   `json:"system_owner" validate:"required"`
	Delegates     []string `json:"delegates,omitempty"`
}

type AccessLevelPostRequest struct {
	Name      string              `json:"access_level_name" validate:"required"`
	ShortName string              `json:"access_level_short_name" validate:"required"`
	Members   *AccessLevelMembers `json:"members,omitempty"`
}

type Container struct {
	ContainerType string         `json:"container_type"`
	DisplayName   string         `json:"display_name"`
	ShortName     string         `json:"short_name"`
	URL           string         `json:"url,omitempty"`
	SystemOwner   string         `json:"system_owner"`
	Delegates     []string       `json:"delegates,omitempty"`
	AccessLevels  []*AccessLevel `json:"access_levels,omitempty"`
}

type AccessLevel struct {
	System          string              `json:"system,omitempty"`
	Name            string              `json:"access_level_name"`
	ShortName       string              `json:"access_level_short_name"`
	ADGroupName     string              `json:"ad_group_name,omitempty"`
	Members         *AccessLevelMembers `json:"members,omitempty"`
	AWSResourceName string              `json:"aws_arn,omitempty"`
}

type AccessLevelMembers struct {
	Users []string `json:"users,omitempty"`
}

type Client interface {
	GetContainer(ctx context.Context, containerName string) (*Container, error)
	PostContainer(ctx context.Context, container *ContainerPostRequest) (*Container, error)
	DeleteContainer(ctx context.Context, containerShortName string) error
	GetContainerAccessLevel(ctx context.Context, containerName, accessLevelName string) (*AccessLevel, error)
	PostAccessLevel(ctx context.Context, containerName string, accessLevel *AccessLevelPostRequest) (*AccessLevel, error)
}

// SSAMClient is the REST client for the SSAM API, kind of like an ORM for the entire API
type clientImpl struct {
	mutator    *restclient.RequestMutator
	httpClient *http.Client
	validator  *validation.Validator
	asap       pkiutil.ASAP
}

// NewSSAMClient creates an SSAMClient. The baseURL is the host of the SSAM API excluding the path but including the
// scheme, i.e. https://ssam.office.atlassian.com/
func NewSSAMClient(httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) Client {
	return &clientImpl{
		mutator: restclient.NewRequestMutator(
			restclient.BaseURL(baseURL.String()),
			pkiutil.AuthenticateWithASAP(asap, asapAudience, asapSubject),
		),
		httpClient: httpClient,
		validator:  validation.New(),
		asap:       asap,
	}
}

var (
	shortNameRegexp = regexp.MustCompile("^[A-Za-z0-9-_]+$")
)

func validateShortName(shortName string) error {
	// This is a manual check, as we will get back a 404 with HTML, and URL escaping (" " => "%20") does not fix it.
	if !shortNameRegexp.MatchString(shortName) {
		return errors.New("Short Name must only contain '-', '_', or alphanumeric chars")
	}

	return nil
}

// GetContainer returns an SSAM Container given the short name of the container. If that container exists that container
// is returned. If something went wrong while getting that container, an error is returned, if it could not be found,
// the error will be signal this.
func (c clientImpl) GetContainer(ctx context.Context, containerShortName string) (*Container, error) {
	logger := logz.RetrieveLoggerFromContext(ctx)
	logger.Debug("SSAM GetContainer", zappers.ContainerShortName(containerShortName))

	if err := validateShortName(containerShortName); err != nil {
		return nil, err
	}

	req, err := c.mutator.NewRequest(
		restclient.Context(ctx),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(urlGetContainer(containerShortName)))
	if err != nil {
		return nil, errors.Wrap(err, "error returned while creating HTTP request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer util.CloseSilently(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, httputil.NewNotFound("container with short-name %q was not found", containerShortName)
		}

		return nil, c.getErrorFromResponse(resp, logger)
	}

	container := new(Container)
	if err := json.NewDecoder(resp.Body).Decode(container); err != nil {
		return nil, errors.Wrap(err, "unable to decode response body as JSON")
	}

	return container, nil
}

// PostContainer creates a container given the container information, an error returns if a container already exists
// with the same name.
func (c clientImpl) PostContainer(ctx context.Context, container *ContainerPostRequest) (*Container, error) {
	logger := logz.RetrieveLoggerFromContext(ctx)
	logger.Debug("SSAM PostContainer",
		zappers.ContainerShortName(container.ShortName),
		zappers.ContainerSystemOwner(container.SystemOwner))

	if err := c.validator.Validate(container); err != nil {
		return nil, err
	}

	if err := validateShortName(container.ShortName); err != nil {
		return nil, err
	}

	req, err := c.mutator.NewRequest(
		restclient.Context(ctx),
		restclient.Method(http.MethodPost),
		restclient.JoinPath(urlCreateContainer()),
		restclient.BodyFromJSON(container))
	if err != nil {
		return nil, errors.Wrap(err, "error returned while creating HTTP request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer util.CloseSilently(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return nil, c.getErrorFromResponse(resp, logger)
	}

	newContainer := new(Container)
	if err := json.NewDecoder(resp.Body).Decode(newContainer); err != nil {
		return nil, errors.Wrap(err, "unable to decode response body as JSON")
	}
	logger.Info("SSAM PostContainer success",
		zappers.ContainerShortName(container.ShortName),
		zappers.ContainerSystemOwner(container.SystemOwner))
	return newContainer, nil
}

// GetContainerAccessLevel gets an access level given the access level short name and container short name
func (c clientImpl) GetContainerAccessLevel(ctx context.Context, containerShortName, accessLevelShortName string) (*AccessLevel, error) {
	logger := logz.RetrieveLoggerFromContext(ctx)
	logger.Debug("SSAM GetContainerAccessLevel",
		zappers.ContainerShortName(containerShortName),
		zappers.AccessLevelShortName(accessLevelShortName))

	if err := validateShortName(containerShortName); err != nil {
		return nil, err
	}

	if err := validateShortName(accessLevelShortName); err != nil {
		return nil, err
	}

	req, err := c.mutator.NewRequest(
		restclient.Context(ctx),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(urlGetContainerAccessLevel(containerShortName, accessLevelShortName)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "error returned while creating http request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, httputil.NewNotFound("access level %q was not found", accessLevelShortName)
		}

		return nil, c.getErrorFromResponse(resp, logger)
	}

	accessLevel := &AccessLevel{}
	if err := json.NewDecoder(resp.Body).Decode(accessLevel); err != nil {
		return nil, errors.Wrap(err, "unable to decode response body as JSON")
	}

	return accessLevel, nil
}

// PostAccessLevel creates an access level for a container given the access level details and the container short name
func (c clientImpl) PostAccessLevel(ctx context.Context, containerShortName string, accessLevel *AccessLevelPostRequest) (*AccessLevel, error) {
	logger := logz.RetrieveLoggerFromContext(ctx)

	logger.Debug("SSAM PostContainerAccessLevel",
		zappers.ContainerShortName(containerShortName),
		zappers.AccessLevelShortName(accessLevel.ShortName),
		zappers.AccessLevelMembers(accessLevel.Members.Users))

	if err := c.validator.Validate(accessLevel); err != nil {
		return nil, err
	}

	if err := validateShortName(containerShortName); err != nil {
		return nil, err
	}

	if err := validateShortName(accessLevel.ShortName); err != nil {
		return nil, err
	}

	req, err := c.mutator.NewRequest(
		restclient.Context(ctx),
		restclient.Method(http.MethodPost),
		restclient.JoinPath(urlCreateContainerAccessLevel(containerShortName)),
		restclient.BodyFromJSON(accessLevel),
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not mutate request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, c.getErrorFromResponse(resp, logger)
	}

	newAccessLevel := &AccessLevel{}
	if err := json.NewDecoder(resp.Body).Decode(newAccessLevel); err != nil {
		return nil, errors.Wrap(err, "unable to decode response body as JSON")
	}

	logger.Info("SSAM PostContainerAccessLevel success",
		zappers.ContainerShortName(containerShortName),
		zappers.AccessLevelShortName(accessLevel.ShortName),
		zappers.AccessLevelMembers(accessLevel.Members.Users))
	return newAccessLevel, nil
}

func (c clientImpl) DeleteContainer(ctx context.Context, containerShortName string) error {
	logger := logz.RetrieveLoggerFromContext(ctx)
	logger.Debug("SSAM GetContainer", zappers.ContainerShortName(containerShortName))

	if err := validateShortName(containerShortName); err != nil {
		logger.Warn("SSAM GetContainer name %q does not obey naming scheme, will attempt to delete anyway",
			zappers.ContainerShortName(containerShortName))
	}

	// Delete the container
	req, err := c.mutator.NewRequest(
		restclient.Context(ctx),
		restclient.Method(http.MethodDelete),
		restclient.JoinPath(urlGetContainer(containerShortName)))
	if err != nil {
		return errors.Wrap(err, "error returned while creating HTTP request")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer util.CloseSilently(resp.Body)

	if resp.StatusCode != http.StatusNoContent {
		if resp.StatusCode == http.StatusNotFound {
			return httputil.NewNotFound("container with short-name %q was not found", containerShortName)
		}

		return c.getErrorFromResponse(resp, logger)
	}

	return nil
}
