package restclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"
)

type RequestMutation func(req *http.Request) (*http.Request, error)

const (
	// ContentTypeJSON is the default for JSON strings
	ContentTypeJSON = "application/json"
)

// Body will correctly take the contents of a Reader and embed it as the body of the request.
func Body(body io.Reader) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		if body == nil {
			req.Body = nil
			return req, nil
		}

		// If the ContentLength is actually 0, we can signal that ContentLength is ACTUALLY 0, and not just unknown
		OptimizeIfEmpty := func(req *http.Request) {
			if req.ContentLength == 0 {

				// This signals that the ContentLengt
				req.Body = http.NoBody
				req.GetBody = func() (io.ReadCloser, error) {
					return http.NoBody, nil
				}
			}
		}

		switch v := body.(type) {
		case *bytes.Buffer:
			req.ContentLength = int64(v.Len())
			buf := v.Bytes()
			req.Body = ioutil.NopCloser(body)
			req.GetBody = func() (io.ReadCloser, error) {
				r := bytes.NewReader(buf)
				return ioutil.NopCloser(r), nil
			}
			OptimizeIfEmpty(req)

		case *bytes.Reader:
			req.ContentLength = int64(v.Len())
			snapshot := *v
			req.Body = ioutil.NopCloser(body)
			req.GetBody = func() (io.ReadCloser, error) {
				r := snapshot
				return ioutil.NopCloser(&r), nil
			}
			OptimizeIfEmpty(req)

		case *strings.Reader:
			req.ContentLength = int64(v.Len())
			snapshot := *v
			req.Body = ioutil.NopCloser(body)
			req.GetBody = func() (io.ReadCloser, error) {
				r := snapshot
				return ioutil.NopCloser(&r), nil
			}
			OptimizeIfEmpty(req)

		default:
			// We don't have the same backwards compatibility issues as req.go:
			// https://github.com/golang/go/blob/226651a541/src/net/http/req.go#L823-L839

			// Convert the body to a ReadCloser
			rc, ok := body.(io.ReadCloser)
			if !ok {
				rc = ioutil.NopCloser(body)
			}
			req.Body = rc
			req.GetBody = func() (io.ReadCloser, error) {
				return rc, nil
			}

			// We don't know how to get the Length, this is signalled by setting ContentLength to 0
			req.ContentLength = 0
		}

		return req, nil
	}
}

// Context adds a context.Context to the request that can be retrieved in handlers receiving the context.
func Context(ctx context.Context) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		return req.WithContext(ctx), nil
	}
}

// BodyFromJSON marshals the interface `v` into JSON for the req
func BodyFromJSON(v interface{}) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		b := new(bytes.Buffer)
		if err := json.NewEncoder(b).Encode(v); err != nil {
			return nil, errors.Wrap(err, "could not encode body")
		}
		req, err := Body(b)(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not set body")
		}
		req.Header.Set("Content-Type", ContentTypeJSON)
		return req, nil
	}
}

// BodyFromJSONString uses the string as JSON for the req
func BodyFromJSONString(s string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		if err := json.Unmarshal([]byte(s), &map[string]interface{}{}); err != nil {
			return nil, errors.Wrap(err, "BodyFromJSONString received string that was not valid JSON")
		}
		req, err := Body(strings.NewReader(s))(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not set body")
		}
		req.Header.Set("Content-Type", ContentTypeJSON)
		return req, nil
	}
}

// Method sets the HTTP method of the request, e.g. http.MethodGet ("GET")
func Method(method string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		req.Method = method
		return req, nil
	}
}

// Header adds a header to the request
func Header(key, value string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		req.Header.Add(key, value)
		return req, nil
	}
}

// BaseURL sets the URL of the req from a URL string
func BaseURL(base string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		u, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		req.URL = u
		return req, nil
	}
}

// ResolvePath attempts to resolve path against the base url, use this when you need to allow merging paths with ..
// relative joins must not start with /
// see https://golang.org/pkg/net/url/#URL.ResolveReference for semantics
func ResolvePath(path string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		u, err := url.Parse(path)
		if err != nil {
			return nil, err
		}
		req.URL.Path = req.URL.ResolveReference(u).Path
		return req, nil
	}
}

// JoinPath takes a relative pathstring and joins it to the end of the current url, does not allow .., preserves tailing /
func JoinPath(pathString string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		if pathString == "" {
			return req, nil
		}
		u, err := url.Parse(pathString)
		if err != nil {
			return nil, err
		}
		if u.IsAbs() {
			return nil, errors.New("pathString must be relative")
		}
		req.URL.Path = path.Join(req.URL.Path, u.Path)
		if strings.HasSuffix(pathString, "/") {
			// preserve ending slash, path.Join removes it
			req.URL.Path += "/"
		}
		return req, nil
	}
}

// Query adds a key and potentially a list of value query param pair without applying query escaping
func Query(key string, values ...string) RequestMutation {
	return func(req *http.Request) (*http.Request, error) {
		if req.URL == nil {
			req.URL = &url.URL{}
		}
		query := req.URL.Query()
		query[key] = append(query[key], values...)
		req.URL.RawQuery = query.Encode()
		return req, nil
	}
}
