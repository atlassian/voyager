package apiservice

import (
	"encoding/json"
	"net/http"

	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	DefaultUsernameHeader     = "X-Remote-User"
	DefaultGroupHeader        = "X-Remote-Group"
	DefaultExtraHeadersPrefix = "X-Remote-Extra-"

	ApiserverAuthenticationNamespace             = "kube-system"
	ApiserverAuthenticationConfigMap             = "extension-apiserver-authentication"
	ApiserverAuthenticationKeyCA                 = "requestheader-client-ca-file"
	ApiserverAuthenticationKeyUsername           = "requestheader-username-headers"
	ApiserverAuthenticationKeyGroup              = "requestheader-group-headers"
	ApiserverAuthenticationKeyExtraHeadersPrefix = "requestheader-extra-headers-prefix"

	ConfigFromConfigMap = "cluster"
	ConfigFromConfig    = "config"
)

type RequestHeaderMiddlewareConfig struct {
	Local               bool     `json:"local"`
	ConfigFrom          string   `json:"configFrom"`
	ClientCA            string   `json:"clientCA"`
	NameHeaders         []string `json:"nameHeaders"`
	GroupHeaders        []string `json:"groupHeaders"`
	ExtraHeaderPrefixes []string `json:"extraHeaderPrefixes"`
}

func (o *RequestHeaderMiddlewareConfig) DefaultAndValidate() []error {
	var allErrors []error

	if o.ConfigFrom == "" || o.ConfigFrom == ConfigFromConfigMap {
		o.ConfigFrom = ConfigFromConfigMap

		// Just read it from the configMap. Make sure everything is empty.
		if len(o.ClientCA) != 0 {
			allErrors = append(allErrors, errors.New("clientCA must not have a value for cluster config"))
		}
		if len(o.NameHeaders) != 0 {
			allErrors = append(allErrors, errors.New("nameHeaders must not have a value for cluster config"))
		}
		if len(o.GroupHeaders) != 0 {
			allErrors = append(allErrors, errors.New("groupHeaders must not have a value for cluster config"))
		}
		if len(o.ExtraHeaderPrefixes) != 0 {
			allErrors = append(allErrors, errors.New("extraHeaderPrefixes must not have a value for cluster config"))
		}
	} else if o.ConfigFrom == ConfigFromConfig {
		if len(o.ClientCA) == 0 {
			allErrors = append(allErrors, errors.New("clientCA must have a value for config"))
		}
		if len(o.NameHeaders) == 0 {
			allErrors = append(allErrors, errors.New("nameHeaders must have a value for config"))
		}
	} else {
		allErrors = append(allErrors, errors.New("invalid configFrom value"))
	}

	return allErrors
}

type RequestHeaderMiddleware struct {
	authenticator *X509RequestHeaderAuthenticator
	local         bool
}

func ConfigFromK8s(mainClient kubernetes.Interface) (*RequestHeaderMiddlewareConfig, error) {
	opts := meta_v1.GetOptions{}
	configMap, err := mainClient.CoreV1().ConfigMaps(ApiserverAuthenticationNamespace).Get(ApiserverAuthenticationConfigMap, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var username []string
	var group []string
	var extraHeaders []string
	clientCA := configMap.Data[ApiserverAuthenticationKeyCA]

	err = json.Unmarshal([]byte(configMap.Data[ApiserverAuthenticationKeyUsername]), &username)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = json.Unmarshal([]byte(configMap.Data[ApiserverAuthenticationKeyGroup]), &group)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = json.Unmarshal([]byte(configMap.Data[ApiserverAuthenticationKeyExtraHeadersPrefix]), &extraHeaders)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &RequestHeaderMiddlewareConfig{
		ClientCA:            clientCA,
		NameHeaders:         username,
		GroupHeaders:        group,
		ExtraHeaderPrefixes: extraHeaders,
	}, nil
}

func ConstructRequestHeaderMiddleware(config *RequestHeaderMiddlewareConfig) (*RequestHeaderMiddleware, error) {
	authenticator, err := NewX509RequestHeaderAuthenticator(
		config.ClientCA,
		config.NameHeaders,
		config.GroupHeaders,
		config.ExtraHeaderPrefixes)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &RequestHeaderMiddleware{
		authenticator: authenticator,
		local:         config.Local,
	}, nil
}

func (r *RequestHeaderMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		logger := logz.RetrieveLoggerFromContext(ctx)
		var authInfo auth.AggregatorUserInfo

		if r.local {
			users := req.Header[DefaultUsernameHeader]
			user := "not-set"
			if len(users) != 0 {
				user = users[0]
			}

			authInfo = auth.AggregatorUserInfo{
				User:   user,
				Groups: req.Header[DefaultGroupHeader],
			}
		} else {
			var ok bool
			var err error

			authInfo, ok, err = r.authenticator.AuthenticateRequest(req)
			if err != nil {
				RespondWithUnauthorizedError(logger, w, req, "Request denied", err)
				return
			}

			if !ok {
				RespondWithUnauthorizedError(logger, w, req, "Missing certificate or username", nil)
				return
			}
		}

		ctx = logz.CreateContextWithLogger(ctx, logger.With(
			zap.String("user", authInfo.User),
			zap.Strings("groups", authInfo.Groups),
		))
		ctx = auth.CreateContextWithAuthInfo(ctx, authInfo)
		newReq := req.WithContext(ctx)
		next.ServeHTTP(w, newReq)
	})
}
