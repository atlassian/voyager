package rps

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"bitbucket.org/atlassianlabs/restclient"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/pkiutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	osbResourcesPath = "/api/v1/resourceType/osb"

	asapAudience = "resource-provisioning-service"
)

type ClientImpl struct {
	logger     *zap.Logger
	httpClient *http.Client
	rm         *restclient.RequestMutator
}

func NewRPSClient(logger *zap.Logger, httpClient *http.Client, asap pkiutil.ASAP, baseURL *url.URL) *ClientImpl {
	rm := restclient.NewRequestMutator(
		restclient.BaseURL(baseURL.String()),
		pkiutil.AuthenticateWithASAP(asap, asapAudience, ""),
	)
	return &ClientImpl{
		logger:     logger,
		httpClient: httpClient,
		rm:         rm,
	}
}

func (c *ClientImpl) ListOSBResources(ctx context.Context) ([]OSBResource, error) {
	req, err := c.rm.NewRequest(
		restclient.Method(http.MethodGet),
		restclient.JoinPath(osbResourcesPath),
		restclient.Header("Content-Type", "application/json"),
		restclient.Context(ctx),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create search")
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute search")
	}

	defer util.CloseSilently(response.Body)
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s: %s", response.Status, respBody)
	}

	var osbResources []OSBResource
	if err = json.Unmarshal(respBody, &osbResources); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal osb resources from RPS")
	}

	return osbResources, nil
}
