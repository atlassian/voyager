package pagerduty

import (
	"testing"

	pagerdutyClient "github.com/PagerDuty/go-pagerduty"
	"github.com/atlassian/voyager"
	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/testutil"
	"github.com/atlassian/voyager/pkg/util/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testUUID                            = "b5dd92da-cc9b-40f4-a854-821d45197288"
	testServiceName voyager.ServiceName = "my-service"
	testEmail                           = "an_owner@example.com"

	testUserID             = "userID"
	testEscalationPolicyID = "escalationID"
	testServiceID          = "serviceID"
	testIntegrationID      = "integrationID"
)

// Making sure that the actual type implements the interface
var (
	_ pagerdutyRestClient = &pagerdutyClient.Client{}
	_ pagerdutyRestClient = &pagerdutyRestClientMock{}
	_ uuid.Generator      = &uuidGeneratorMock{}
)

type uuidGeneratorMock struct {
	mock.Mock
}

func (m *uuidGeneratorMock) NewUUID() string {
	args := m.Called()
	return args.Get(0).(string)
}

type pagerdutyRestClientMock struct {
	mock.Mock
}

func (m *pagerdutyRestClientMock) ListEscalationPolicies(o pagerdutyClient.ListEscalationPoliciesOptions) (*pagerdutyClient.ListEscalationPoliciesResponse, error) {
	args := m.Called(o)
	return args.Get(0).(*pagerdutyClient.ListEscalationPoliciesResponse), args.Error(1)
}

func (m *pagerdutyRestClientMock) ListUsers(o pagerdutyClient.ListUsersOptions) (*pagerdutyClient.ListUsersResponse, error) {
	args := m.Called(o)
	return args.Get(0).(*pagerdutyClient.ListUsersResponse), args.Error(1)
}

func (m *pagerdutyRestClientMock) ListServices(o pagerdutyClient.ListServiceOptions) (*pagerdutyClient.ListServiceResponse, error) {
	args := m.Called(o)
	return args.Get(0).(*pagerdutyClient.ListServiceResponse), args.Error(1)
}

func (m *pagerdutyRestClientMock) CreateEscalationPolicy(e pagerdutyClient.EscalationPolicy) (*pagerdutyClient.EscalationPolicy, error) {
	args := m.Called(e)
	return args.Get(0).(*pagerdutyClient.EscalationPolicy), args.Error(1)
}

func (m *pagerdutyRestClientMock) CreateService(s pagerdutyClient.Service) (*pagerdutyClient.Service, error) {
	args := m.Called(s)
	return args.Get(0).(*pagerdutyClient.Service), args.Error(1)
}

func (m *pagerdutyRestClientMock) CreateIntegration(id string, i pagerdutyClient.Integration) (*pagerdutyClient.Integration, error) {
	args := m.Called(id, i)
	return args.Get(0).(*pagerdutyClient.Integration), args.Error(1)
}

func (m *pagerdutyRestClientMock) CreateUser(u pagerdutyClient.User) (*pagerdutyClient.User, error) {
	args := m.Called(u)
	return args.Get(0).(*pagerdutyClient.User), args.Error(1)
}

func (m *pagerdutyRestClientMock) DeleteEscalationPolicy(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *pagerdutyRestClientMock) DeleteService(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

var (
	testUsername = testutil.Named("nislamov")
)

func TestGetServiceSearchURL(t *testing.T) {
	t.Parallel()
	// given
	const serviceName voyager.ServiceName = "nislamov-creator"
	// when
	url, err := GetServiceSearchURL(serviceName)
	// then
	require.NoError(t, err)
	assert.Equal(t, `https://atlassian.pagerduty.com/services#?query=Micros%202%20-%20nislamov-creator`, url)
}

func TestFindOrCreate(t *testing.T) {
	t.Parallel()
	// given
	uuidGeneratorMock := new(uuidGeneratorMock)
	uuidGeneratorMock.On("NewUUID").Return(testUUID)
	pagerdutyRestClient := new(pagerdutyRestClientMock)
	// ListUsers
	pagerdutyRestClient.On("ListUsers", pagerdutyClient.ListUsersOptions{
		Query: testEmail,
	}).Return(&pagerdutyClient.ListUsersResponse{}, nil)
	// CreateUser
	pagerdutyRestClient.On("CreateUser", *newUser(false)).
		Return(newUser(true), nil)
	// ListEscalationPolicies
	pagerdutyRestClient.On("ListEscalationPolicies", pagerdutyClient.ListEscalationPoliciesOptions{
		Query: string(nameEscalationPolicy(testServiceName)),
	}).Return(&pagerdutyClient.ListEscalationPoliciesResponse{}, nil)
	// CreateEscalationPolicy
	pagerdutyRestClient.On("CreateEscalationPolicy", *newEscalationPolicy(false)).Return(newEscalationPolicy(true), nil)
	// ListServices
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{}, nil)
	// CreateService
	pagerdutyRestClient.On("CreateService", *newService(voyager.EnvTypeProduction, false, false, false)).
		Return(newService(voyager.EnvTypeProduction, false, true, false), nil)
	pagerdutyRestClient.On("CreateService", *newService(voyager.EnvTypeProduction, true, false, false)).
		Return(newService(voyager.EnvTypeProduction, true, true, false), nil)
	pagerdutyRestClient.On("CreateService", *newService(voyager.EnvTypeStaging, false, false, false)).
		Return(newService(voyager.EnvTypeStaging, false, true, false), nil)
	pagerdutyRestClient.On("CreateService", *newService(voyager.EnvTypeStaging, true, false, false)).
		Return(newService(voyager.EnvTypeStaging, true, true, false), nil)
	// CreateIntegration
	pagerdutyRestClient.On("CreateIntegration", testServiceID, *newGenericIntegration(false)).Return(newGenericIntegration(true), nil)
	pagerdutyRestClient.On("CreateIntegration", testServiceID, *newPingdomIntegration(false)).Return(newPingdomIntegration(true), nil)
	pagerdutyRestClient.On("CreateIntegration", testServiceID, *newCloudWatchIntegration(false)).Return(newCloudWatchIntegration(true), nil)

	creator, err := New(zaptest.NewLogger(t), pagerdutyRestClient, uuidGeneratorMock)
	require.NoError(t, err)
	// when
	meta, err := creator.FindOrCreate(testServiceName, testUsername, testEmail)
	// then
	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, newMetadata(), meta)
}

func TestFindOrCreateSucceedsIfAlreadyExists(t *testing.T) {
	t.Parallel()
	// given
	uuidGeneratorMock := new(uuidGeneratorMock)
	uuidGeneratorMock.On("NewUUID").Return(testUUID)
	pagerdutyRestClient := new(pagerdutyRestClientMock)
	// ListUsers
	pagerdutyRestClient.On("ListUsers", pagerdutyClient.ListUsersOptions{
		Query: testEmail,
	}).Return(&pagerdutyClient.ListUsersResponse{
		Users: []pagerdutyClient.User{*newUser(true)},
	}, nil)
	// ListEscalationPolicies
	pagerdutyRestClient.On("ListEscalationPolicies", pagerdutyClient.ListEscalationPoliciesOptions{
		Query: string(nameEscalationPolicy(testServiceName)),
	}).Return(&pagerdutyClient.ListEscalationPoliciesResponse{
		EscalationPolicies: []pagerdutyClient.EscalationPolicy{*newEscalationPolicy(true)},
	}, nil)
	// ListServices
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeProduction, true, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeProduction, false, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeStaging, true, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeStaging, false, true, true),
		},
	}, nil)

	pagerduty, err := New(zaptest.NewLogger(t), pagerdutyRestClient, uuidGeneratorMock)
	require.NoError(t, err)
	// when
	meta, err := pagerduty.FindOrCreate(testServiceName, testUsername, testEmail)
	// then
	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, newMetadata(), meta)
}

func TestDelete(t *testing.T) {
	t.Parallel()
	// given
	uuidGeneratorMock := new(uuidGeneratorMock)
	uuidGeneratorMock.On("NewUUID").Return(testUUID)
	pagerdutyRestClient := new(pagerdutyRestClientMock)
	// ListUsers
	pagerdutyRestClient.On("ListUsers", pagerdutyClient.ListUsersOptions{
		Query: testEmail,
	}).Return(&pagerdutyClient.ListUsersResponse{
		Users: []pagerdutyClient.User{*newUser(true)},
	}, nil)
	// ListEscalationPolicies
	pagerdutyRestClient.On("ListEscalationPolicies", pagerdutyClient.ListEscalationPoliciesOptions{
		Query: string(nameEscalationPolicy(testServiceName)),
	}).Return(&pagerdutyClient.ListEscalationPoliciesResponse{
		EscalationPolicies: []pagerdutyClient.EscalationPolicy{*newEscalationPolicy(true)},
	}, nil)
	// ListServices
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeProduction, true, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeProduction, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeProduction, false, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, true)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeStaging, true, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("ListServices", pagerdutyClient.ListServiceOptions{
		Query:    string(nameService(testServiceName, voyager.EnvTypeStaging, false)),
		Includes: []string{"integrations"},
	}).Return(&pagerdutyClient.ListServiceResponse{
		Services: []pagerdutyClient.Service{
			*newService(voyager.EnvTypeStaging, false, true, true),
		},
	}, nil)
	pagerdutyRestClient.On("DeleteService", testServiceID).Return(nil)
	pagerdutyRestClient.On("DeleteEscalationPolicy", testEscalationPolicyID).Return(nil)

	pagerduty, err := New(zaptest.NewLogger(t), pagerdutyRestClient, uuidGeneratorMock)
	require.NoError(t, err)
	// when
	err = pagerduty.Delete(testServiceName)
	// then
	require.NoError(t, err)
}

func TestDeleteLeavesEscalationPolicy(t *testing.T) {
	t.Parallel()
	// given
	uuidGeneratorMock := new(uuidGeneratorMock)
	uuidGeneratorMock.On("NewUUID").Return(testUUID)
	pagerdutyRestClient := new(pagerdutyRestClientMock)
	policy := *newEscalationPolicy(true)
	policy.Services = []pagerdutyClient.APIReference{
		{ID: testServiceID},
	}

	// ListUsers
	pagerdutyRestClient.On("ListUsers", pagerdutyClient.ListUsersOptions{
		Query: testEmail,
	}).Return(&pagerdutyClient.ListUsersResponse{
		Users: []pagerdutyClient.User{*newUser(true)},
	}, nil)
	// ListEscalationPolicies
	pagerdutyRestClient.On("ListEscalationPolicies", pagerdutyClient.ListEscalationPoliciesOptions{
		Query: string(nameEscalationPolicy(testServiceName)),
	}).Return(&pagerdutyClient.ListEscalationPoliciesResponse{
		EscalationPolicies: []pagerdutyClient.EscalationPolicy{policy},
	}, nil)
	// No services here
	pagerdutyRestClient.On("ListServices", mock.Anything).Return(
		&pagerdutyClient.ListServiceResponse{Services: []pagerdutyClient.Service{}}, nil,
	)
	pagerdutyRestClient.On("DeleteService", testServiceID).Return(nil)

	pagerduty, err := New(zaptest.NewLogger(t), pagerdutyRestClient, uuidGeneratorMock)
	require.NoError(t, err)
	// when
	err = pagerduty.Delete(testServiceName)
	// then
	require.NoError(t, err)
}

func newUser(setID bool) *pagerdutyClient.User {
	u := pagerdutyClient.User{
		Name:  testUsername.Name(),
		Email: testEmail,
	}
	if setID {
		u.APIObject = pagerdutyClient.APIObject{
			ID: testUserID,
		}
	}
	return &u
}

func newEscalationPolicy(setID bool) *pagerdutyClient.EscalationPolicy {
	p := pagerdutyClient.EscalationPolicy{
		Name: string(nameEscalationPolicy(testServiceName)),
		EscalationRules: []pagerdutyClient.EscalationRule{
			{
				Delay: 30, // minutes
				Targets: []pagerdutyClient.APIObject{
					{
						Type: "user",
						ID:   testUserID,
					},
				},
			},
		},
	}
	if setID {
		p.APIObject = pagerdutyClient.APIObject{
			ID: testEscalationPolicyID,
		}
	}
	return &p
}

func newService(envType voyager.EnvType, lowPriority bool, setID bool, includeIntegrations bool) *pagerdutyClient.Service {
	urgency := "high"
	if lowPriority {
		urgency = "low"
	}
	s := pagerdutyClient.Service{
		Name: string(nameService(testServiceName, envType, lowPriority)),
		EscalationPolicy: pagerdutyClient.EscalationPolicy{
			APIObject: pagerdutyClient.APIObject{
				ID:   testEscalationPolicyID,
				Type: "escalation_policy_reference",
			},
		},
		IncidentUrgencyRule: &pagerdutyClient.IncidentUrgencyRule{
			Type:    "constant",
			Urgency: urgency,
		},
	}
	if setID {
		s.APIObject = pagerdutyClient.APIObject{
			ID: testServiceID,
		}
	}
	if includeIntegrations {
		s.Integrations = []pagerdutyClient.Integration{
			*newGenericIntegration(true),
			*newPingdomIntegration(true),
			*newCloudWatchIntegration(true),
		}
	}
	return &s
}

func newIntegration(name IntegrationName, setID bool) *pagerdutyClient.Integration {
	i := pagerdutyClient.Integration{
		Name: string(name),
	}
	if setID {
		i.APIObject = pagerdutyClient.APIObject{
			ID: testIntegrationID,
		}
		i.IntegrationKey = string(name) + "-key"
	}
	return &i
}

func newGenericIntegration(setID bool) *pagerdutyClient.Integration {
	i := newIntegration(Generic, setID)
	i.Type = "generic_events_api_inbound_integration"
	return i
}

func newCloudWatchIntegration(setID bool) *pagerdutyClient.Integration {
	i := newIntegration(CloudWatch, setID)
	i.Type = "aws_cloudwatch_inbound_integration"
	i.Vendor = &pagerdutyClient.APIObject{
		Type: "vendor_reference",
		ID:   awsInternalID,
	}
	return i
}

func newPingdomIntegration(setID bool) *pagerdutyClient.Integration {
	i := newIntegration(Pingdom, setID)
	i.Type = "pingdom_inbound_integration"
	i.IntegrationEmail = testUUID
	return i
}

func newMetadata() creator_v1.PagerDutyMetadata {
	sm := creator_v1.PagerDutyServiceMetadata{
		ServiceID: testServiceID,
		PolicyID:  testEscalationPolicyID,
		Integrations: creator_v1.PagerDutyIntegrations{
			Generic: creator_v1.PagerDutyIntegrationMetadata{
				IntegrationID:  testIntegrationID,
				IntegrationKey: string(Generic) + "-key",
			},
			CloudWatch: creator_v1.PagerDutyIntegrationMetadata{
				IntegrationID:  testIntegrationID,
				IntegrationKey: string(CloudWatch) + "-key",
			},
			Pingdom: creator_v1.PagerDutyIntegrationMetadata{
				IntegrationID:  testIntegrationID,
				IntegrationKey: string(Pingdom) + "-key",
			},
		},
	}
	return creator_v1.PagerDutyMetadata{
		Production: creator_v1.PagerDutyEnvMetadata{
			Main:        sm,
			LowPriority: sm,
		},
		Staging: creator_v1.PagerDutyEnvMetadata{
			Main:        sm,
			LowPriority: sm,
		},
	}
}
