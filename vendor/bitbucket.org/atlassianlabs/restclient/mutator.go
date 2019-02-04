package restclient

import (
	"net/http"
)

type RequestMutator struct {
	mutations []RequestMutation
}

// NewRequestMutator manages RequestMutations. It's main purpose is to store a base set of mutations and then apply them
// to a http.Request (or new http.Request) with extra mutations, always mutating in the order given.
func NewRequestMutator(commonMutations ...RequestMutation) *RequestMutator {
	return &RequestMutator{
		mutations: commonMutations,
	}
}

// Mutate will mutate the request with the common mutations and the given mutations, in the order provided.
func (rm *RequestMutator) Mutate(req *http.Request, mutations ...RequestMutation) (*http.Request, error) {
	mutations = append(rm.mutations, mutations...)
	for _, mutation := range mutations {
		var err error
		req, err = mutation(req)
		if err != nil {
			return nil, err
		}
	}
	return req, nil
}

// NewRequest will create a blank http.Request and then call Mutate(newrequest, mutations)
func (rm *RequestMutator) NewRequest(mutations ...RequestMutation) (*http.Request, error) {
	req, err := http.NewRequest("", "", nil)
	if err != nil {
		return nil, err
	}

	req, err = rm.Mutate(req, mutations...)
	if err != nil {
		return nil, err
	}

	return req, nil
}
