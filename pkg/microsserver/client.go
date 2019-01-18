package microsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	noUser       = ""
	asapAudience = "micros-server"
)

// Client is a minimalistic Micros Server client for our needs
// See full API spec https://stash.atlassian.com/projects/MICROS/repos/micros-server/browse/schema/swagger.yaml
type Client struct {
	logger     *zap.Logger
	httpClient *http.Client
	asap       pkiutil.ASAP
	rm         *restclient.RequestMutator
}

func NewMicrosServerClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *Client {
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

func (c *Client) GetAlias(ctx context.Context, domainName string) (*AliasInfo, error) {
	getAliasEndpoint := "/api/v1/aliases"
	req, err := c.rm.NewRequest(
		pkiutil.AuthenticateWithASAP(c.asap, asapAudience, noUser),
		restclient.Method(http.MethodGet),
		restclient.JoinPath(getAliasEndpoint),
		restclient.Query("domainName", domainName),
		restclient.Context(ctx),
		restclient.Header("Accept", "application/json"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create get alias request")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute get alias request")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if response.StatusCode != http.StatusOK {
		message := fmt.Sprintf("failed to fetch service for %q. Response: %s", domainName, respBody)
		return nil, clientError(response.StatusCode, message)
	}

	var parsedBody AliasInfo
	err = json.Unmarshal(respBody, &parsedBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	return &parsedBody, nil
}
