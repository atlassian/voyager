package releases

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/atlassian/voyager/pkg/releases/deployinator/models"
	"github.com/pact-foundation/pact-go/dsl"
)

/**
This test tests the consumer interactions surrounding the resolve endpoints between
voyager (consumer) and trebuchet (provider).

To validate the pact against the current provider specs locally, use the contract testing cli found here:
https://hello.atlassian.net/wiki/spaces/TESTA/pages/128609304/Contract+Testing+Getting+Started+Guide+for+Consumers
 */
func TestConsumerPact(t *testing.T) {

	// Initialize the pact
	pact := &dsl.Pact{
		Consumer: "voyager",
		Provider: "deployinator-trebuchet",
		Host:     "localhost",
		PactDir:  "generated/pacts",
	}

	defer pact.Teardown()

	// Define the interactions in resolve and batchResolve separately, with the same pact

	ResolveEndpointTest(pact)

	BatchResolveEndpointTest(pact)

	// Once interactions are verified successfully,
	// a pact contract between the voyager/trebuchet should have now been generated. See contract under pacts/
}

func ResolveEndpointTest(pact *dsl.Pact) {
	singleReleaseGroup := models.ResolutionResponseType{
		Label:         "",
		Service:       "ServiceName",
		ReleaseGroups: map[string]map[string]interface{}{},
	}

	// Define request with dummy variables and headers set
	var resolveTest = func() error {
		u := fmt.Sprintf("http://localhost:%d/api/v1/resolve", pact.Server.Port)
		req, err := http.NewRequest("GET", u,
			strings.NewReader(
				`{"service":"serviceName", "environment":"dev", "region":"us-east", "account": "1233"}`))

		req.Header.Set("Content-Type", "application/json")
		if err != nil {
			return err
		}
		if _, err = http.DefaultClient.Do(req); err != nil {
			return err
		}

		return err
	}

	// Define the expected response
	pact.
		AddInteraction().
		Given("There is a release group that matches the service and location given").
		UponReceiving("A request to get the release group").
		WithRequest(dsl.Request{
			Method:  "GET",
			Path:    dsl.String("/api/v1/resolve"),
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json")},
		}).
		WillRespondWith(dsl.Response{
			Status:  200,
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json")},
			Body:    singleReleaseGroup,
		})

	// Verify the interaction
	if err := pact.Verify(resolveTest); err != nil {
		log.Fatalf("Error on Vertify for resolve endpoint: %v", err)
	}

}

func BatchResolveEndpointTest(pact *dsl.Pact) {
	singleReleaseGroup := models.ResolutionResponseType{
		Label:         "",
		Service:       "ServiceName",
		ReleaseGroups: map[string]map[string]interface{}{},
	}

	var batchResolveTest = func() error {
		u := fmt.Sprintf("http://localhost:%d/api/v1/resolve/batch", pact.Server.Port)
		req, err := http.NewRequest("GET", u,
			strings.NewReader(`{"environment":"dev", "region":"us-east", "account": "1233"}`))

		req.Header.Set("Content-Type", "application/json")
		if err != nil {
			return err
		}
		if _, err = http.DefaultClient.Do(req); err != nil {
			return err
		}

		return err
	}

	pact.
		AddInteraction().
		Given("").
		UponReceiving("A request to get all the release groups under location").
		WithRequest(dsl.Request{
			Method:  "GET",
			Path:    dsl.String("/api/v1/resolve/batch"),
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json")},
		}).
		WillRespondWith(dsl.Response{
			Status:  200,
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json")},
			Body: models.BatchResolutionResponseType{
				NextFrom:    "from",
				NextTo:      "to",
				PageDetails: &models.PageDetails{Page: 1, PageCount: 1, Total: 1},
				Results:     []*models.ResolutionResponseType{&singleReleaseGroup},
			},
		})

	if err := pact.Verify(batchResolveTest); err != nil {
		log.Fatalf("Error on Vertify for resolve endpoint: %v", err)
	}

}
