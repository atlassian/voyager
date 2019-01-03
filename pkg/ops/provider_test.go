package ops

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SermoDigital/jose/jws"
	ops_v1 "github.com/atlassian/voyager/pkg/apis/ops/v1"
	. "github.com/atlassian/voyager/pkg/util/httputil/httptest"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

type MockASAPConfig struct{}

func (*MockASAPConfig) GenerateToken(audience string, subject string) ([]byte, error) {
	return []byte("ASAP Token"), nil
}

func (*MockASAPConfig) GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error) {
	return []byte("ASAP Token"), nil
}

func (*MockASAPConfig) KeyID() string     { return "" }
func (*MockASAPConfig) KeyIssuer() string { return "" }

type MockFailedASAPConfig struct{}

func (*MockFailedASAPConfig) GenerateToken(audience string, subject string) ([]byte, error) {
	return []byte{}, errors.New("Failed to sign request for some reason")
}

func (*MockFailedASAPConfig) GenerateTokenWithClaims(audience string, subject string, claims jws.Claims) ([]byte, error) {
	return []byte{}, errors.New("Failed to sign request for some reason")
}

func (*MockFailedASAPConfig) KeyID() string     { return "" }
func (*MockFailedASAPConfig) KeyIssuer() string { return "" }

const (
	dummySwagger = `
{
  "openapi": "3.0.0",
  "info": {
    "version": "0.0.1-alpha",
    "title": "Dummy schema",
    "contact": {
      "email": "micros@atlassian.com"
    }
  },
  "servers": [
    {
      "description": "dev",
      "url": "https://micros.dev.atl-paas.net"
    },
    {
      "description": "prod",
      "url": "https://micros.prod.atl-paas.net"
    }
  ],
  "paths": {
    "/v2/service_instances/{instance_id}/x-operation_instances/scale": {
      "post": {
        "operationId":"scale",
        "summary": "Scale the compute",
        "parameters": [
          {
            "name": "instance_id",
            "description": "The instance to be acted upon",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": null,
              "type": "object",
              "description": "Represents parameters for a scale",
              "additionalProperties": false,
              "properties": {
                "min": {
                  "type": "integer"
                },
                "max": {
                  "type": "integer"
                },
                "desired": {
                  "type": "integer"
                },
                "group": {
                  "type": "string"
                }
              },
              "required": [
                "group"
              ]
            }
          }
        },
        "responses": {
          "200": {
            "description": "Operation completed successfully. See Response Body.",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/result"
                }
              }
            }
          },
          "202": {
            "description": "Job was created and will be executed asynchronously. See Response Body.",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/jobReference"
                }
              }
            }
          },
          "default": {
            "description": "unexpected error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/errors"
                }
              }
            }
          }
        }
      }
    },
    "/v2/service_instances/{instance_id}/x-operation_instances/terminate_instance": {
      "post": {
        "operationId":"terminate_instance",
        "summary": "Scale the compute",
        "parameters": [
          {
            "name": "instance_id",
            "description": "The instance to be acted upon",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "description": "Represents parameters for a scale",
                "additionalProperties": false,
                "properties": {
                  "ec2InstanceId": {
                    "type": "string",
                    "example": "i-1234567890ABCDEF"
                  }
                },
                "required": [
                  "ec2InstanceId"
                ]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Operation completed successfully. See Response Body.",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/result"
                }
              }
            }
          },
          "202": {
            "description": "Job was created and will be executed asynchronously. See Response Body.",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/jobReference"
                }
              }
            }
          },
          "default": {
            "description": "unexpected error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/errors"
                }
              }
            }
          }
        }
      }
    },
    "/v2/service_instances/{instance_id}/x-operation_instances/info": {
      "get": {
        "summary": "get info",
				"tags": ["report"],
				"operationId": "info",
        "parameters": [
          {
            "name": "instance_id",
            "description": "The instance to be acted upon",
            "in": "path",
            "required": true,
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Return report",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/result"
                }
              }
            }
          },
          "default": {
            "description": "unexpected error",
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/errors"
                }
              }
            }
          }
        }
      }
    }
	}
}`
	dummyScaleResponse = `
	{
		"scale": "IN_PROGRESS"
	}`
	dummyScaleResult = `
	{
		"scale": "COMPLETE"
	}`
)

func createMockHandler() *HTTPMock {
	return MockHandler(
		Match(Get, Path(discoveryURI)).Respond(
			Status(http.StatusOK),
			Body(dummySwagger),
		),
		Match(Post, Path("/v2/service_instances/uuid/x-operation_instances/scale")).Respond(
			Status(http.StatusAccepted),
			Body(dummyScaleResponse),
		),
		Match(Get, Path("/v2/service_instances/uuid/x-job_instances/job_uuid/result")).Respond(
			Status(http.StatusOK),
			Body(dummyScaleResult),
		),
	)
}

func createMissingMockServer() *HTTPMock {
	return MockHandler(
		Match(Get, Path(discoveryURI)).Respond(
			Status(http.StatusNotFound),
		),
	)
}

func TestHandlesHappyCreation(t *testing.T) {
	t.Parallel()
	handler := createMockHandler()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)

	assert.Equal(t, &Provider{
		ProviderName: "ec2provider",
		audience:     "micros-server",

		actions: provider.actions,

		client: provider.client,

		baseURL: provider.baseURL,
	}, provider)

	assert.ElementsMatch(t, provider.actions, []Action{
		Action{
			action: "scale",
			method: "post",
		},
		Action{
			action: "terminate_instance",
			method: "post",
		},
		Action{
			action: "info",
			tags:   []string{"report"},
			method: "get",
		},
	})
}

func TestHandlesMissingSchema(t *testing.T) {
	t.Parallel()

	handler := createMissingMockServer()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, _, err := NewProvider(zaptest.NewLogger(t), broker)
	require.Error(t, err, "Attempted to get provider schema returned status code: 404")
	require.Equal(t, 1, handler.ReqestSnapshots.Calls())
}

func MockProxyRequest(t *testing.T, method string, requestURI string) *http.Request {
	req, err := http.NewRequest(
		method,
		requestURI,
		strings.NewReader("requestBody"),
	)

	require.NoError(t, err)

	// Instead of faking the TLS cert and sending through our middleware chain
	// this pretends everything is correctly set (except for logging).
	ctx := logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t))
	ctx = context.WithValue(ctx, middleware.RequestIDKey, requestId)
	ctx = request.WithUser(ctx, &user.DefaultInfo{
		Name:   "user",
		Groups: []string{"groupA", "groupB"},
	})

	return req.WithContext(ctx)
}

func TestProxyPostRequestSuccessfully(t *testing.T) {
	t.Parallel()

	handler := createMockHandler()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)

	req := MockProxyRequest(t, http.MethodPost, "ops-gateway/namespace/voyager/ec2provider/uuid/x-operation_instances/scale")
	parsedURI := "/v2/service_instances/uuid/x-operation_instances/scale"

	recorder := httptest.NewRecorder()
	provider.ProxyRequest(&MockASAPConfig{}, recorder, req, parsedURI)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	require.Equal(t, dummyScaleResponse, string(body))
	require.Equal(t, 2, handler.ReqestSnapshots.Calls())
}

func TestSetupReportCorrectly(t *testing.T) {
	t.Parallel()

	handler := createMockHandler()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)

	require.Equal(t, "info", provider.ReportAction())
}

func TestProxyGetRequestSuccessfully(t *testing.T) {
	t.Parallel()

	handler := createMockHandler()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)

	req := MockProxyRequest(t, http.MethodGet, "ops-gateway/namespace/voyager/ec2provider/uuid/x-job_instances/job_uuid/result")
	parsedURI := "/v2/service_instances/uuid/x-job_instances/job_uuid/result"

	recorder := httptest.NewRecorder()
	provider.ProxyRequest(&MockASAPConfig{}, recorder, req, parsedURI)

	require.Equal(t, http.StatusOK, recorder.Code)
	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	require.Equal(t, dummyScaleResult, string(body))
	require.Equal(t, 2, handler.ReqestSnapshots.Calls())
}

func TestHandleASAPFailure(t *testing.T) {
	t.Parallel()

	handler := createMockHandler()
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "ec2provider",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL,
			ASAP: ops_v1.ASAPSpec{
				Audience: "micros-server",
			},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)

	req := MockProxyRequest(t, http.MethodGet, "ops-gateway/namespace/voyager/ec2provider/uuid/x-job_instances/job_uuid/result")
	parsedURI := "/v2/service_instances/uuid/x-job_instances/job_uuid/result"

	recorder := httptest.NewRecorder()
	provider.ProxyRequest(&MockFailedASAPConfig{}, recorder, req, parsedURI)

	require.EqualValues(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	opsErrors := meta_v1.Status{}

	body, err := ioutil.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &opsErrors)
	require.NoError(t, err)
	require.Equal(t, opsErrors.Message, "some-request-id: error making request to provider ec2provider: Error setting up asap with provider: Failed to sign request for some reason")
	require.EqualValues(t, opsErrors.Code, http.StatusInternalServerError)
	require.Equal(t, 1, handler.ReqestSnapshots.Calls())
}

func TestRouteWithContextPath(t *testing.T) {
	t.Parallel()

	handler := MockHandler(
		Match(Get, Path("/context"+discoveryURI)).Respond(
			Status(http.StatusOK),
			Body(dummySwagger),
		),
	)
	mockServer := httptest.NewServer(handler)
	defer mockServer.Close()

	broker := &ops_v1.Route{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "context-broker",
		},
		Spec: ops_v1.RouteSpec{
			URL: mockServer.URL + "/context",
			ASAP: ops_v1.ASAPSpec{
				Audience: "context-broker",
			},
			Plans: []string{"aaaaaaaa-3500-454a-87cd-aaaaaaaaaaaa"},
		},
	}

	_, provider, err := NewProvider(zaptest.NewLogger(t), broker)
	require.NoError(t, err)
	require.Equal(t, "/context", provider.baseURL.Path)
	require.Equal(t, []string{"aaaaaaaa-3500-454a-87cd-aaaaaaaaaaaa"}, provider.plans)
	require.Equal(t, "context-broker", provider.audience)
	require.Len(t, provider.actions, 3)

}
