package releases

import (
	"testing"
	"time"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/releases/deployinator/client"
	"github.com/atlassian/voyager/pkg/releases/deployinator/client/resolve"
	"github.com/atlassian/voyager/pkg/releases/deployinator/models"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockedTransport struct {
	mock.Mock
}

func (m *mockedTransport) Submit(operation *runtime.ClientOperation) (interface{}, error) {
	args := m.Called(operation)
	return args.Get(0), args.Error(1)
}

func TestShouldRespondWithResultOnOK(t *testing.T) {
	t.Parallel()

	mockedTransport := new(mockedTransport)
	rms := NewReleaseManagement(client.New(mockedTransport, strfmt.NewFormats()), zap.NewNop())
	okResult := resolve.NewResolveOK()
	expectedReleaseData := struct {
		digest string
		image  string
	}{"sha256:2328d00bde49bff6a4d251607da37866e605f27add5ca16f40a99a9824a5e0df", "trebuchet"}

	okResult.Payload = &models.ResolutionResponseType{ReleaseGroups: map[string]map[string]interface{}{
		"deployinator": {"trebuchet": expectedReleaseData},
	}}
	mockedTransport.On("Submit", mock.Anything).Return(okResult, nil)

	res, err := rms.Resolve(ResolveParams{
		Environment: "dev", ServiceName: "svc-name", Region: "us-west-1", Account: "12345",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, expectedReleaseData, res.ResolvedData["deployinator"]["trebuchet"])
}

func TestShouldNotReturnErrorOnNotFound(t *testing.T) {
	t.Parallel()

	mockedTransport := new(mockedTransport)
	rms := NewReleaseManagement(client.New(mockedTransport, strfmt.NewFormats()), zap.NewNop())
	notFoundResult := resolve.NewResolveNotFound()
	mockedTransport.On("Submit", mock.Anything).Return(nil, notFoundResult)

	res, err := rms.Resolve(ResolveParams{
		Environment: "dev", ServiceName: "svc-name", Region: "us-west-1", Account: "12345",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ResolvedRelease{
		ServiceName: "svc-name",
		Label:       "",
	}, *res)
}

func TestBatchResolveShouldRespondWithResultWithOnePage(t *testing.T) {
	t.Parallel()

	mockedTransport := new(mockedTransport)
	rms := NewReleaseManagement(client.New(mockedTransport, strfmt.NewFormats()), zap.NewNop())
	okResult := resolve.NewResolveBatchOK()
	noContentResult := resolve.NewResolveBatchNoContent()
	expectedReleaseData := struct {
		digest string
		image  string
	}{"sha256:2328d00bde49bff6a4d251607da37866e605f27add5ca16f40a99a9824a5e0df", "trebuchet"}
	expectedNextFrom := time.Now().Format(time.RFC3339)

	okResult.Payload = &models.BatchResolutionResponseType{
		PageDetails: &models.PageDetails{
			Total:     3,
			Page:      0,
			PageCount: 1,
		},
		NextFrom: time.Now().Format(time.RFC3339),
		NextTo:   expectedNextFrom,
		Results: []*models.ResolutionResponseType{
			{
				Service: "svc-name1",
				Label:   "",
				ReleaseGroups: map[string]map[string]interface{}{
					"rg1": {"alias1": expectedReleaseData},
				},
			},
			{
				Service: "svc-name1",
				Label:   "labelA",
				ReleaseGroups: map[string]map[string]interface{}{
					"rg1": {"alias1": expectedReleaseData},
				},
			},
			{
				Service: "svc-name2",
				Label:   "",
				ReleaseGroups: map[string]map[string]interface{}{
					"rg1": {"alias1": expectedReleaseData},
				},
			},
		},
	}
	mockedTransport.On("Submit", mock.Anything).Return(okResult, nil).Once()
	mockedTransport.On("Submit", mock.Anything).Return(noContentResult, nil).Once()

	res, nextFrom, err := rms.ResolveLatest(ResolveBatchParams{
		Environment: "dev", Region: "us-west-1", Account: "12345", From: time.Time{},
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, nextFrom)
	assert.Equal(t, expectedReleaseData, res[0].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name1", res[0].ServiceName)
	assert.Equal(t, voyager.Label(""), res[0].Label)
	assert.Equal(t, expectedReleaseData, res[1].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name1", res[1].ServiceName)
	assert.Equal(t, voyager.Label("labelA"), res[1].Label)
	assert.Equal(t, expectedReleaseData, res[2].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name2", res[2].ServiceName)
	assert.Equal(t, voyager.Label(""), res[2].Label)
}

func TestBatchResolveShouldAggregateResultsOverMultiplePages(t *testing.T) {
	t.Parallel()

	mockedTransport := new(mockedTransport)
	rms := NewReleaseManagement(client.New(mockedTransport, strfmt.NewFormats()), zap.NewNop())
	expectedReleaseData := struct {
		digest string
		image  string
	}{"sha256:2328d00bde49bff6a4d251607da37866e605f27add5ca16f40a99a9824a5e0df", "trebuchet"}
	expectedResponseStartTime := time.Unix(100, 0)

	makePayload := func(page int32, serviceName string, label string) *models.BatchResolutionResponseType {
		return &models.BatchResolutionResponseType{
			PageDetails: &models.PageDetails{
				Total:     3,
				Page:      page,
				PageCount: 3,
			},
			NextFrom: time.Unix(0, 0).Format(time.RFC3339),
			NextTo:   expectedResponseStartTime.Format(time.RFC3339),
			Results: []*models.ResolutionResponseType{
				{
					Service: serviceName,
					Label:   label,
					ReleaseGroups: map[string]map[string]interface{}{
						"rg1": {"alias1": expectedReleaseData},
					},
				},
			},
		}
	}

	noContentResult := resolve.NewResolveBatchNoContent()
	page1 := resolve.NewResolveBatchOK()
	page1.Payload = makePayload(0, "svc-name1", "")
	page2 := resolve.NewResolveBatchOK()
	page2.Payload = makePayload(1, "svc-name1", "labelA")
	page3 := resolve.NewResolveBatchOK()
	page3.Payload = makePayload(2, "svc-name2", "")

	mockedTransport.On("Submit", mock.Anything).Return(page1, nil).Once()
	mockedTransport.On("Submit", mock.Anything).Return(page2, nil).Once()
	mockedTransport.On("Submit", mock.Anything).Return(page3, nil).Once()
	mockedTransport.On("Submit", mock.Anything).Return(noContentResult, nil).Once()

	startFrom := time.Unix(0, 0)
	res, nextFrom, err := rms.ResolveLatest(ResolveBatchParams{
		Environment: "dev", Region: "us-west-1", Account: "12345", From: startFrom,
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, nextFrom)
	assert.Equal(t, expectedResponseStartTime.UTC(), nextFrom.UTC())
	assert.Equal(t, expectedReleaseData, res[0].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name1", res[0].ServiceName)
	assert.Equal(t, voyager.Label(""), res[0].Label)
	assert.Equal(t, expectedReleaseData, res[1].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name1", res[1].ServiceName)
	assert.Equal(t, voyager.Label("labelA"), res[1].Label)
	assert.Equal(t, expectedReleaseData, res[2].ResolvedData["rg1"]["alias1"])
	assert.Equal(t, "svc-name2", res[2].ServiceName)
	assert.Equal(t, voyager.Label(""), res[2].Label)
}

func TestBatchResolveShouldHandleNoContentResult(t *testing.T) {
	t.Parallel()

	mockedTransport := new(mockedTransport)
	rms := NewReleaseManagement(client.New(mockedTransport, strfmt.NewFormats()), zap.NewNop())
	noContentResult := resolve.NewResolveBatchNoContent()

	mockedTransport.On("Submit", mock.Anything).Return(noContentResult, nil).Once()

	expectedNextFrom := time.Now()
	res, nextFrom, err := rms.ResolveLatest(ResolveBatchParams{
		Environment: "dev", Region: "us-west-1", Account: "12345", From: expectedNextFrom,
	})

	require.NoError(t, err)
	require.Nil(t, res)
	require.NotNil(t, nextFrom)
	assert.Equal(t, expectedNextFrom, nextFrom)
}
