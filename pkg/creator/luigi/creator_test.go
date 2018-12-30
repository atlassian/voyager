package luigi

import (
	"context"
	"testing"

	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// Making sure that the actual type implements the interface
var _ Client = &ClientImpl{}

type luigiClientMock struct {
	mock.Mock
}

func (m *luigiClientMock) CreateService(ctx context.Context, data *FullServiceData) (*FullServiceData, error) {
	args := m.Called(ctx, data)
	return args.Get(0).(*FullServiceData), args.Error(1)
}

func (m *luigiClientMock) ListServices(ctx context.Context, search string) ([]BasicServiceData, error) {
	args := m.Called(ctx, search)
	return args.Get(0).([]BasicServiceData), args.Error(1)
}

func (m *luigiClientMock) DeleteService(ctx context.Context, loggingID string) error {
	args := m.Called(ctx, loggingID)
	return args.Error(0)
}

func TestCreateService(t *testing.T) {
	t.Parallel()
	result := newTestServiceData(true)
	// given
	luigiClient := new(luigiClientMock)
	luigiClient.On("CreateService", mock.Anything, newInputTestServiceData()).Return(result, nil)
	creator := NewCreator(zaptest.NewLogger(t), luigiClient)
	// when
	data, err := creator.FindOrCreateService(context.Background(), newServiceMetadata())
	// then
	require.NoError(t, err)
	assert.Equal(t, &result.BasicServiceData, data)
}

func TestCreateServiceSucceedsIfServiceWithTheSameDataExistsCreator(t *testing.T) {
	t.Parallel()
	// given
	testServiceData := newTestServiceData(true)
	listServiceData := newListServiceData(&testServiceData.BasicServiceData)
	luigiClient := new(luigiClientMock)
	luigiClient.On("CreateService", mock.Anything, mock.Anything).Return((*FullServiceData)(nil), httputil.NewConflict("already exists"))
	luigiClient.On("ListServices", mock.Anything, "test-service").Return(listServiceData, nil)
	creator := NewCreator(zaptest.NewLogger(t), luigiClient)
	// when
	data, err := creator.FindOrCreateService(context.Background(), newServiceMetadata())
	// then
	require.NoError(t, err)
	assert.Equal(t, &testServiceData.BasicServiceData, data)
}

func TestCreateServiceFailsIfServiceWithTheSameNameButDifferentOwnerExistsCreator(t *testing.T) {
	t.Parallel()
	// given
	testServiceData := newTestServiceData(true)
	testServiceData.Owner = "somebody-else"
	listServiceData := newListServiceData(&testServiceData.BasicServiceData)
	luigiClient := new(luigiClientMock)
	luigiClient.On("CreateService", mock.Anything, mock.Anything).Return((*FullServiceData)(nil), httputil.NewConflict("already exists"))
	luigiClient.On("ListServices", mock.Anything, "test-service").Return(listServiceData, nil)
	creator := NewCreator(zaptest.NewLogger(t), luigiClient)
	// when
	_, err := creator.FindOrCreateService(context.Background(), newServiceMetadata())
	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), `expected owner "an_owner" returned "somebody-else"`)
}

func TestCreateServiceFailsIfServiceExistsButCouldNotBeFound(t *testing.T) {
	t.Parallel()
	// given
	luigiClient := new(luigiClientMock)
	luigiClient.On("CreateService", mock.Anything, mock.Anything).Return((*FullServiceData)(nil), httputil.NewConflict("already exists"))
	luigiClient.On("ListServices", mock.Anything, "test-service").Return([]BasicServiceData{}, nil)
	creator := NewCreator(zaptest.NewLogger(t), luigiClient)
	// when
	_, err := creator.FindOrCreateService(context.Background(), newServiceMetadata())
	// then
	require.Error(t, err) // no panic
}

func newServiceMetadata() *ServiceMetadata {
	return &ServiceMetadata{
		BusinessUnit: "some_unit",
		Name:         "test-service",
		Owner:        "an_owner",
	}
}

func newListServiceData(actual *BasicServiceData) []BasicServiceData {
	return []BasicServiceData{
		BasicServiceData{
			SourceID:          "micros",
			Name:              "test-service-test-service",
			Organization:      "Engineering Services",
			Owner:             "an_owner",
			Admins:            "",
			CapacityGigabytes: 1,
			CapacityComment:   "default capacity",
		},
		BasicServiceData{
			SourceID:          "micros2",
			Name:              "my-test-service",
			Organization:      "some_unit",
			Owner:             "an_owner",
			Admins:            "",
			CapacityGigabytes: 1,
			CapacityComment:   "wow",
		},
		*actual,
		BasicServiceData{
			SourceID:          "memes",
			Name:              "test-service-12",
			Organization:      "Identity",
			Owner:             "mcb",
			Admins:            "",
			CapacityGigabytes: 1024,
			CapacityComment:   "need more space",
		},
	}
}

func newInputTestServiceData() *FullServiceData {
	s := FullServiceData{
		BasicServiceData: BasicServiceData{
			SourceID:     "micros2",
			Name:         "test-service",
			Organization: "some_unit",
			Owner:        "an_owner",
			Admins:       "",
		},
		Acls: []ServiceACL{
			ServiceACL{Environments: "*", StaffIDGroup: "atlassian-staff"},
		},
	}
	return &s
}
