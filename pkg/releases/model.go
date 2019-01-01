package releases

import (
	"time"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/releases/deployinator/client"
	"github.com/atlassian/voyager/pkg/releases/deployinator/client/resolve"
	"github.com/atlassian/voyager/pkg/releases/deployinator/models"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// DefaultReleaseMetadataConfigMapName is the name of the configmap each service will be able to find its release data in
	DefaultReleaseMetadataConfigMapName = "service-release"
	// DataKey is the name of the key to store resolved releases data from release management system within the configmap
	DataKey = "releases"
)

// ResolveParams structure to hold the parameters required for release management system to resolve release data
type ResolveParams struct {
	Account     voyager.Account
	Environment voyager.EnvType
	Label       voyager.Label
	Region      voyager.Region
	ServiceName voyager.ServiceName
}

type ResolveBatchParams struct {
	Account     voyager.Account
	Environment voyager.EnvType
	Region      voyager.Region
	From        time.Time
}

type resolveBatchPageParams struct {
	ResolveBatchParams
	To   *time.Time
	Page int32
}

// ResolvedRelease represents the response of a release management system
type ResolvedReleaseData map[string]map[string]interface{}

type ResolvedRelease struct {
	ResolvedData ResolvedReleaseData
	ServiceName  voyager.ServiceName
	Label        voyager.Label
}

// ReleaseManagementStore generic interface to request the release data from RMS
type ReleaseManagementStore interface {
	// Resolve returns release data grouped by a release context
	Resolve(params ResolveParams) (*ResolvedRelease, error)

	// Resolve all services that have been updated in given time ranges in batches (paginated)
	ResolveLatest(params ResolveBatchParams) ([]ResolvedRelease, time.Time, error)
}

// DeployinatorRMS default implementation of ReleaseManagementStore interface
type DeployinatorRMS struct {
	Logger       *zap.Logger
	Deployinator *client.Deployinator
}

// NewReleaseManagement creates new release management store object
func NewReleaseManagement(depHTTPClient *client.Deployinator, logger *zap.Logger) ReleaseManagementStore {
	return &DeployinatorRMS{Deployinator: depHTTPClient, Logger: logger}
}

func (rms *DeployinatorRMS) Resolve(params ResolveParams) (*ResolvedRelease, error) {
	rms.Logger.Info("Resolve release data for", logz.ServiceName(params.ServiceName),
		logz.Account(params.Account), logz.Region(params.Region), logz.EnvType(params.Environment),
		logz.Label(params.Label))
	account := string(params.Account)
	envType := string(params.Environment)
	label := string(params.Label)
	region := string(params.Region)
	name := string(params.ServiceName)
	response, err := rms.Deployinator.Resolve.Resolve(resolve.NewResolveParams().
		WithAccount(&account).
		WithEnvironment(&envType).
		WithLabel(&label).
		WithRegion(&region).
		WithService(&name))
	if err != nil {
		if _, ok := err.(*resolve.ResolveNotFound); ok {
			return &ResolvedRelease{
				ServiceName: params.ServiceName,
				Label:       voyager.Label(label),
			}, nil
		}
		return nil, err
	}
	return &ResolvedRelease{
		ResolvedData: response.Payload.ReleaseGroups,
		ServiceName:  voyager.ServiceName(response.Payload.Service),
		Label:        voyager.Label(response.Payload.Label),
	}, nil
}

func (rms *DeployinatorRMS) ResolveLatest(params ResolveBatchParams) ([]ResolvedRelease, time.Time /* Next "from" param to use */, error) {
	rms.Logger.Info("Resolving release batches",
		logz.Account(params.Account), logz.Region(params.Region), logz.EnvType(params.Environment))

	request := resolveBatchPageParams{
		ResolveBatchParams: ResolveBatchParams{
			Environment: params.Environment,
			Account:     params.Account,
			Region:      params.Region,
			From:        params.From,
		},
		Page: 0,
	}

	res, err := rms.resolveSingleBatch(request)
	if err != nil {
		return nil, params.From, err
	}
	var results []ResolvedRelease
	for res != nil {
		for _, v := range res.Results {
			resolvedRelease := ResolvedRelease{
				ResolvedData: v.ReleaseGroups,
				ServiceName:  voyager.ServiceName(v.Service),
				Label:        voyager.Label(v.Label),
			}
			results = append(results, resolvedRelease)
		}

		nextFromValue, err := time.Parse(time.RFC3339, res.NextFrom)
		if err != nil {
			return nil, params.From, errors.Errorf("invalid next from date format returned: %s, RFC3339 format expected", res.NextFrom)
		}
		nextToValue, err := time.Parse(time.RFC3339, res.NextTo)
		if err != nil {
			return nil, params.From, errors.Errorf("invalid next to date format returned: %s, RFC3339 format expected", res.NextTo)
		}
		request.From = nextFromValue
		request.To = &nextToValue
		request.Page = res.PageDetails.Page + 1
		res, err = rms.resolveSingleBatch(request)
		if err != nil {
			return nil, params.From, err
		}
	}

	if len(results) == 0 {
		// No content, return the same 'From' time for the next request
		return nil, request.From, nil
	}
	return results, *request.To, nil
}

func (rms *DeployinatorRMS) resolveSingleBatch(params resolveBatchPageParams) (*models.BatchResolutionResponseType, error) {
	account := string(params.Account)
	envType := string(params.Environment)
	region := string(params.Region)

	requestParams := resolve.NewResolveBatchParams().
		WithAccount(&account).
		WithEnvironment(&envType).
		WithRegion(&region).
		WithPage(&params.Page)

	formattedFrom := params.From.Format(time.RFC3339)
	requestParams = requestParams.WithFrom(&formattedFrom)
	if params.To != nil {
		formattedTo := params.To.Format(time.RFC3339)
		requestParams = requestParams.WithTo(&formattedTo)
	}

	response, noContent, err := rms.Deployinator.Resolve.ResolveBatch(requestParams)
	if err != nil {
		return nil, err
	}
	if noContent != nil {
		return nil, nil
	}

	return response.Payload, nil
}
