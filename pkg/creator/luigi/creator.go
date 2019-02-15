package luigi

import (
	"context"

	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	voyagerPlatform = "micros2" // Marking services owned by Voyager

	defaultACLEnvironment  = "*"
	defaultACLStaffIDGroup = "atlassian-staff"
)

type ServiceMetadata struct {
	Name         string
	BusinessUnit string
	Owner        string
	Admins       string
}

type Creator struct {
	logger *zap.Logger
	client Client
}

func (m *ServiceMetadata) toServiceData(platform string, acls []ServiceACL) *FullServiceData {
	return &FullServiceData{
		BasicServiceData: BasicServiceData{
			SourceID:     platform,
			Name:         m.Name,
			Organization: m.BusinessUnit,
			Owner:        m.Owner,
			Admins:       m.Admins,
		},
		Acls: append([]ServiceACL(nil), acls...),
	}
}

func NewCreator(logger *zap.Logger, client Client) *Creator {
	return &Creator{
		logger: logger,
		client: client,
	}
}

func (c *Creator) FindOrCreateService(ctx context.Context, meta *ServiceMetadata) (*BasicServiceData, error) {
	data := meta.toServiceData(voyagerPlatform, []ServiceACL{
		{
			Environments: defaultACLEnvironment,
			StaffIDGroup: defaultACLStaffIDGroup,
		},
	})

	responseData, err := c.client.CreateService(ctx, data)
	if err == nil {
		return &responseData.BasicServiceData, nil
	}

	if httputil.IsConflict(err) {
		var actual *BasicServiceData
		actual, err = c.findExistingService(ctx, meta)
		if err != nil {
			return nil, errors.Wrapf(err, "conflicting service %q already exists", meta.Name)
		}

		c.logger.Sugar().Infof("service %q already exists in Luigi, reusing it", meta.Name)
		return actual, nil
	}

	return nil, err
}

func (c *Creator) findExistingService(ctx context.Context, meta *ServiceMetadata) (*BasicServiceData, error) {
	serviceDatas, err := c.client.ListServices(ctx, meta.Name)
	if err != nil {
		return nil, err
	}

	// luigi uses the search string to find everything
	// containing the string. We need to do some filtering.
	var found *BasicServiceData
	for _, v := range serviceDatas {
		if v.Name == meta.Name {
			found = &v
			break
		}
	}

	if found == nil {
		return nil, errors.Errorf("could not find luigi service %q", meta.Name)
	}

	// Match the service data to ensure we found the right thing
	if meta.BusinessUnit != found.Organization {
		return nil, errors.Errorf("expected organization %q returned %q", meta.BusinessUnit, found.Organization)
	}

	if meta.Owner != found.Owner {
		return nil, errors.Errorf("expected owner %q returned %q", meta.Owner, found.Owner)
	}

	if found.SourceID != voyagerPlatform {
		return nil, errors.Errorf("expected sourceId %q returned %q", voyagerPlatform, found.SourceID)
	}

	return found, nil
}

func (c *Creator) DeleteService(ctx context.Context, loggingID string) error {
	return c.client.DeleteService(ctx, loggingID)
}
