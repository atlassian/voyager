package ops

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"bitbucket.org/atlassianlabs/restclient"
	ctrllogz "github.com/atlassian/ctrl/logz"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	"github.com/atlassian/voyager/pkg/ops/util/zappers"
	"github.com/atlassian/voyager/pkg/pkiutil"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/apiservice"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
)

const (
	discoveryURI = "/v2/x-operations"

	reportTag = "report"
)

type ProviderInterface interface {
	ProxyRequest(asapConfig pkiutil.ASAP, w http.ResponseWriter, r *http.Request, uri string)
	Request(asapConfig pkiutil.ASAP, r *http.Request, uri string, user string) (*http.Response, error)
	ReportAction() string
	Render(w http.ResponseWriter, r *http.Request) error

	Name() string
	OwnsPlan(string) bool
}

type Action struct {
	method string
	action string
	tags   []string // Currently tags are only for `get` methods as reporting is the only consumer
}

type PathDefinition struct {
	Tags        []string `json:"tags"`
	OperationID string   `json:"operationId"`
}

type Provider struct {
	ProviderName string `json:"name"`
	plans        []string
	audience     string
	actions      []Action
	client       *http.Client
	baseURL      *url.URL
}

// https://extranet.atlassian.com/display/VDEV/Voyager+Ops+API+Specification#VoyagerOpsAPISpecification-Discovery
func NewProvider(logger *zap.Logger, broker *ops_v1.Route) (bool /*retriable*/, *Provider, error) {
	providerLog := loggerForProvider(logger, broker)
	providerLog.Sugar().Infof("Handling new provider: %s, URL: %s", broker.Name, broker.Spec.URL)

	baseURL, err := url.Parse(broker.Spec.URL)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to resolve broker url")
	}
	rm := createProviderClient(broker.Spec.URL)
	providerReq, err := rm.NewRequest(restclient.JoinPath(discoveryURI))
	if err != nil {
		return false, nil, err
	}

	client := util.HTTPClient()
	res, err := client.Do(providerReq)
	if err != nil {
		return true, nil, err
	}

	defer util.CloseSilently(res.Body)

	if res.StatusCode != http.StatusOK {
		return false, nil, errors.Errorf("attempted to get provider schema returned status code: %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, nil, err
	}

	s := OpenAPISpec{}
	err = json.Unmarshal(body, &s)
	if err != nil {
		providerLog.Sugar().Debug("Could not unmarshall response: %s", body)
		return false, nil, err
	}

	p := &Provider{
		ProviderName: broker.ObjectMeta.Name,
		plans:        broker.Spec.Plans,
		audience:     broker.Spec.ASAP.Audience,
		actions:      []Action{},
		client:       client,
		baseURL:      baseURL,
	}

	for path := range s.Paths {
		if strings.Contains(path, "/x-operation_instances/") {
			taggedPath := map[string]PathDefinition{}
			pathMap, ok := s.Paths[path].(map[string]interface{})
			if ok {
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(pathMap, &taggedPath); err == nil {
					if len(taggedPath) > 0 {
						for method, definition := range taggedPath {
							p.actions = append(p.actions, Action{
								method: method,
								action: definition.OperationID,
								tags:   definition.Tags,
							})
						}
					}
				}
			}
		}
	}
	return false, p, nil
}

func (p *Provider) Request(asapConfig pkiutil.ASAP, r *http.Request, uri string, user string) (*http.Response, error) {
	headerValue, err := asapConfig.GenerateToken(p.audience, user)
	if err != nil {
		return nil, errors.Wrap(err, "Error setting up asap with provider")
	}

	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", headerValue))

	relative, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	// modify the input request url to <broker>/<operation>
	r.URL = p.baseURL.ResolveReference(relative)
	return p.client.Do(r)
}

func (p *Provider) ProxyRequest(asapConfig pkiutil.ASAP, w http.ResponseWriter, r *http.Request, uri string) {
	logger := logz.RetrieveLoggerFromContext(r.Context())
	providerRequest, err := http.NewRequest(r.Method, "", r.Body)
	if err != nil {
		apiservice.RespondWithInternalError(logger, w, r, fmt.Sprintf("error returned creating request to provider %s: %s", p.Name(), err.Error()), err)
		return
	}

	userInfo, ok := request.UserFrom(r.Context())
	if !ok {
		apiservice.RespondWithInternalError(logger, w, r, fmt.Sprintf("auth information missing from context"), errors.New("auth information missing from context"))
		return
	}

	providerRequest = providerRequest.WithContext(r.Context())
	resp, err := p.Request(asapConfig, providerRequest, uri, userInfo.GetName())
	if err != nil {
		apiservice.RespondWithInternalError(logger, w, r, fmt.Sprintf("error making request to provider %s: %s", p.Name(), err.Error()), err)
		return
	}
	defer util.CloseSilently(resp.Body)

	logger.Sugar().Debug("Provider request returned", zap.String("Status", resp.Status))

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		logger.Error("failed to copy response body", zap.Error(err))
	}
}

func (p *Provider) Name() string {
	return p.ProviderName
}

func loggerForProvider(logger *zap.Logger, route *ops_v1.Route) *zap.Logger {
	return logger.With(
		zappers.Route(route),
		ctrllogz.Namespace(route),
	)
}

func createProviderClient(url string) *restclient.RequestMutator {
	return restclient.NewRequestMutator(
		restclient.BaseURL(url),
		restclient.Header("Content-Type", "application/json"),
	)
}

func (p *Provider) ReportAction() string {
	for _, v := range p.actions {
		for _, t := range v.tags {
			if t == reportTag {
				return v.action
			}
		}
	}
	return ""
}

func (p *Provider) OwnsPlan(planID string) bool {
	for _, val := range p.plans {
		if val == planID {
			return true
		}
	}
	return false
}
