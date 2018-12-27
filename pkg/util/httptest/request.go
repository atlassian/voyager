package httptest

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

var (
	NoBody     Matcher = &noBodyMatcher{}
	AnyRequest Matcher = &anyRequestMatcher{}

	Get    = Method(http.MethodGet)
	Patch  = Method(http.MethodPatch)
	Post   = Method(http.MethodPost)
	Put    = Method(http.MethodPut)
	Delete = Method(http.MethodDelete)
)

type noBodyMatcher struct{}
type anyRequestMatcher struct{}

func (n *noBodyMatcher) Match(r *http.Request) bool {
	data, err := ioutil.ReadAll(r.Body)
	if err == nil {
		return len(data) == 0
	}
	return false
}

func (*anyRequestMatcher) Match(r *http.Request) bool {
	return true
}

type jsonMatcher struct {
	input interface{}
	typ   reflect.Type
	t     *testing.T
}

func (j *jsonMatcher) Match(r *http.Request) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		return false
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false
	}

	if len(body) == 0 {
		return j.input == nil
	}

	reqJSON, err := unmarshalToType(body, j.typ)
	if err != nil {
		j.t.Logf("failed to parse json: %q with err: %s", body, err)
		return false
	}

	if !cmp.Equal(j.input, reqJSON) {
		j.t.Log(cmp.Diff(j.input, reqJSON))
		return false
	}
	return true
}

func unmarshalToType(body []byte, typ reflect.Type) (interface{}, error) {
	if typ.Kind() != reflect.Ptr {
		return nil, errors.Errorf("input type must be a pointer, got %s", typ)
	}

	// reflect.New return a pointer to a zero'd value
	// use typ.Elem to get *typ rather than **typ back from reflect.New
	reqJSON := reflect.New(typ.Elem()).Interface()

	if err := json.Unmarshal(body, reqJSON); err != nil {
		return nil, err
	}

	return reqJSON, nil
}

// match if the request body would deserialize from json into the input type and be equal
// input is used for comparison, it will not have unmarshalled into it
func JSONof(t *testing.T, input interface{}) Matcher {

	// reflect.TypeOf(zeroValue) get the type of zeroValue returning an reflect interface (not a real interface)
	typeOf := reflect.TypeOf(input)
	if typeOf == nil {
		return &jsonMatcher{input: nil, t: t}
	}

	return &jsonMatcher{input: input, typ: typeOf, t: t}
}

type QueryMatcher struct {
	values map[string][]string
	exact  bool
}

func (q *QueryMatcher) Add(key, value string) *QueryMatcher {
	q.values[key] = append(q.values[key], value)
	return q
}

func (q *QueryMatcher) AddValues(key string, values ...string) *QueryMatcher {
	q.values[key] = append(q.values[key], values...)
	return q
}

// ExactMatch requires query values to match exactly (no extra keys/values accepted)
func (q *QueryMatcher) ExactMatch() *QueryMatcher {
	q.exact = true
	return q
}

func Query(key, value string) *QueryMatcher {
	return &QueryMatcher{values: map[string][]string{key: {value}}}
}

func QueryValues(key string, values ...string) *QueryMatcher {
	return &QueryMatcher{values: map[string][]string{key: values}}
}

// Match ignores order for query parameters, exact only cares there are no extra params present
func (q *QueryMatcher) Match(r *http.Request) bool {
	query := r.URL.Query()

	if q.exact && len(q.values) != len(query) {
		return false
	}
	for k, p := range q.values {
		p2 := query[k]
		if q.exact && len(p) != len(p2) {
			return false
		}
		for _, v := range p {
			if !contains(p2, v) {
				return false
			}
		}
	}

	return true
}

type MethodMatcher struct {
	method string
}

func (m *MethodMatcher) Match(r *http.Request) bool {
	return m.method == r.Method

}

func Method(method string) *MethodMatcher {
	return &MethodMatcher{method: method}
}

type PathMatcher struct {
	path string
}

func Path(path string) *PathMatcher {
	return &PathMatcher{path: path}
}

func (p *PathMatcher) Match(r *http.Request) bool {
	return p.path == r.URL.Path
}

func (s *RequestSnapshotter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Add(r)
	w.WriteHeader(StatusNext) // indicate this handler doesn't match
}

type Matcher interface {
	Match(*http.Request) bool
}

type Matchers []Matcher

func Match(m ...Matcher) Matchers {
	return Matchers(m)
}

func contains(list []string, key string) bool {
	for _, value := range list {
		if value == key {
			return true
		}
	}
	return false
}
