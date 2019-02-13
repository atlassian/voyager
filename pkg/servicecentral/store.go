package servicecentral

import (
	"context"
	"fmt"
	"time"

	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	voyagerPlatform    = "micros2" // Marking services owned by Voyager
	defaultServiceTier = 3
)

type serviceCentralClient interface {
	CreateService(ctx context.Context, user auth.User, data *ServiceDataWrite) (*ServiceDataRead, error)
	ListServices(ctx context.Context, user auth.OptionalUser, search string) ([]ServiceDataRead, error)
	ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]ServiceDataRead, error)
	GetService(ctx context.Context, user auth.OptionalUser, serviceUUID string) (*ServiceDataRead, error)
	PatchService(ctx context.Context, user auth.User, data *ServiceDataWrite) error
	DeleteService(ctx context.Context, user auth.User, serviceUUID string) error
}

// Store allows storing and retrieving data from ServiceCentral
type Store struct {
	logger *zap.Logger
	client serviceCentralClient
}

func NewStore(logger *zap.Logger, client serviceCentralClient) *Store {
	return &Store{
		logger: logger,
		client: client,
	}
}

func (c *Store) FindOrCreateService(ctx context.Context, user auth.User, service *creator_v1.Service) (*creator_v1.Service, error) {
	data, err := prepareServiceToWrite(ServiceDataRead{}, service)
	if err != nil {
		return nil, err
	}
	fillServiceDataDefaults(data)
	actual, err := c.client.CreateService(ctx, user, data)
	if err == nil {
		return serviceDataToService(actual)
	}

	if httputil.IsConflict(err) {
		actual, err = c.validateExistingService(ctx, auth.ToOptionalUser(user), service.Spec.ResourceOwner, data)
		if err != nil {
			return nil, errors.Wrapf(err, "conflicting service %q already exists", data.ServiceName)
		}

		c.logger.Sugar().Infof("service %q already exists in Service Central, reusing it", service.Name)
		return serviceDataToService(actual)
	}

	return nil, err
}

func (c *Store) ListServices(ctx context.Context, user auth.OptionalUser) ([]creator_v1.Service, error) {
	search := fmt.Sprintf("platform='%s'", voyagerPlatform)
	serviceDatas, err := c.client.ListServices(ctx, user, search)
	if err != nil {
		return nil, err
	}
	services := make([]creator_v1.Service, len(serviceDatas))
	for i, serviceData := range serviceDatas {
		svc, err := serviceDataToService(&serviceData)
		if err != nil {
			return nil, err
		}
		services[i] = *svc
	}
	return services, nil
}

// ListModifiedServices returns a list of services changed since a specific time
// NB: this will not return all fields, if you need additional data (e.g. compliance or other misc data)
// you will need to make a separate call to GetService for the full object
func (c *Store) ListModifiedServices(ctx context.Context, user auth.OptionalUser, modifiedSince time.Time) ([]creator_v1.Service, error) {
	serviceDatas, err := c.client.ListModifiedServices(ctx, user, modifiedSince)
	if err != nil {
		return nil, err
	}

	services := make([]creator_v1.Service, len(serviceDatas))
	for i, serviceData := range serviceDatas {
		if serviceData.Platform != voyagerPlatform {
			// ListModifiedServices doesn't support filtering by platform, so we have to do filtering on the client side :(
			// Feature request: https://sdog.jira-dev.com/browse/MICROSCOPE-280
			continue
		}
		svc, err := serviceDataToService(&serviceData)
		if err != nil {
			return nil, err
		}
		services[i] = *svc
	}
	return services, nil
}

func (c *Store) PatchService(ctx context.Context, user auth.User, service *creator_v1.Service) error {
	existingData, err := c.getServiceDataByName(ctx, auth.ToOptionalUser(user), ServiceName(service.Name))
	if err != nil {
		return err
	}
	updatedData, err := prepareServiceToWrite(*existingData, service)
	if err != nil {
		return err
	}
	updatedData.ServiceUUID = existingData.ServiceUUID
	return c.client.PatchService(ctx, user, updatedData)
}

// GetService retrieves a service from ServiceCentral by name. It only searches for services of type micros2.
func (c *Store) GetService(ctx context.Context, user auth.OptionalUser, name ServiceName) (*creator_v1.Service, error) {
	data, err := c.getServiceDataByName(ctx, user, name)
	if err != nil {
		return nil, err
	}
	creatorV1Service, err := serviceDataToService(data)
	if err != nil {
		return nil, err
	}

	return creatorV1Service, nil
}

func (c *Store) DeleteService(ctx context.Context, user auth.User, name ServiceName) error {
	data, err := c.getServiceDataByName(ctx, auth.ToOptionalUser(user), name)
	if err != nil {
		return err
	}

	err = c.client.DeleteService(ctx, user, *data.ServiceUUID)
	if err != nil {
		if httputil.IsNotFound(err) {
			return NewNotFound(err.Error())
		}
		return err
	}
	return nil
}

func (c *Store) getServiceDataByName(ctx context.Context, user auth.OptionalUser, name ServiceName) (*ServiceDataRead, error) {
	search := fmt.Sprintf("service_name='%s' AND platform='%s'", name, voyagerPlatform)
	listData, err := c.client.ListServices(ctx, user, search)

	if err != nil {
		return nil, errors.Wrapf(err, "error looking up service %q", name)
	}

	// Service Central can return multiple results for a single
	// 'service_name' request (it's either a substring/startswith).
	for _, serviceData := range listData {
		if serviceData.ServiceName == name {
			return c.getServiceDataByUUID(ctx, user, *serviceData.ServiceUUID)
		}
	}

	return nil, NewNotFound("service %q was not found", name)
}

func (c *Store) getServiceDataByUUID(ctx context.Context, user auth.OptionalUser, uuid string) (*ServiceDataRead, error) {
	data, err := c.client.GetService(ctx, user, uuid)

	if err != nil {
		if httputil.IsNotFound(err) {
			return nil, NewNotFound("service with uuid %q was not found", uuid)
		}
		return nil, errors.Wrapf(err, "error getting service %q", data.ServiceName)
	}

	return data, nil
}

func (c *Store) validateExistingService(ctx context.Context, user auth.OptionalUser, originalOwner string, data *ServiceDataWrite) (*ServiceDataRead, error) {
	search := fmt.Sprintf("service_name='%s'", data.ServiceName)
	result, err := c.client.ListServices(ctx, user, search)
	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, errors.Errorf("no single result for service_name %q which supposedly conflicts; got %d", data.ServiceName, len(result))
	}

	existingService := result[0]
	if existingService.ServiceName != data.ServiceName {
		return nil, errors.Errorf("searching for %q returned %q", data.ServiceName, existingService.ServiceName)
	}
	if existingService.ServiceOwner.Username != originalOwner {
		return nil, errors.Errorf("%q not allowed to use service owned by %q", originalOwner, existingService.ServiceOwner.Username)
	}
	if existingService.Platform != voyagerPlatform {
		return nil, errors.Errorf("invalid service platform: expected=%q, actual=%q", voyagerPlatform, existingService.Platform)
	}

	return c.getServiceDataByUUID(ctx, user, *existingService.ServiceUUID)
}

func fillServiceDataDefaults(data *ServiceDataWrite) {
	if data.ServiceTier == 0 {
		data.ServiceTier = defaultServiceTier
	}
	data.ZeroDowntimeUpgrades = true
	data.Stateless = true
	data.Platform = voyagerPlatform
}

// prepareServiceToWrite creates a valid service data for writing to Service Central
// Note: It cannot set Service Owner see: VYGR-425
func prepareServiceToWrite(existingData ServiceDataRead, service *creator_v1.Service) (*ServiceDataWrite, error) {
	serviceUID := string(service.GetUID())
	var serviceUUID *string
	if len(serviceUID) != 0 {
		serviceUUID = &serviceUID
	}

	sd := ServiceDataWrite{
		ServiceUUID:        serviceUUID,
		ServiceName:        ServiceName(service.Name),
		BusinessUnit:       service.Spec.BusinessUnit,
		SSAMContainerName:  service.Spec.SSAMContainerName,
		LoggingID:          service.Spec.LoggingID,
		PagerDutyServiceID: service.Spec.PagerDutyServiceID,
		Misc:               existingData.Misc,
	}

	if service.Spec.Metadata.PagerDuty != nil {
		err := SetPagerDutyMetadata(&sd, service.Spec.Metadata.PagerDuty)
		if err != nil {
			return nil, err
		}
	}

	if service.Spec.Metadata.Bamboo != nil {
		err := SetBambooMetadata(&sd, service.Spec.Metadata.Bamboo)
		if err != nil {
			return nil, err
		}
	}

	// we'll need tags if we're setting them, or there were some already
	if len(existingData.Tags) > 0 || len(service.Spec.ResourceTags) > 0 {
		// we want the combination of non-platform tags (which will filter out previous platform values)
		// and our new set of platform tags, converted into the correct format
		nonPlatformTags := nonPlatformTags(existingData.Tags)
		platformTags := convertPlatformTags(service.Spec.ResourceTags)
		sd.Tags = append(nonPlatformTags, platformTags...) // nolint: gocritic
	}

	return &sd, nil
}

func serviceDataToService(data *ServiceDataRead) (*creator_v1.Service, error) {
	var serviceUID string
	if data.ServiceUUID != nil {
		serviceUID = *data.ServiceUUID
	}

	service := &creator_v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: creator_v1.ServiceResourceAPIVersion,
			Kind:       creator_v1.ServiceResourceKind,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: string(data.ServiceName),
			UID:  types.UID(serviceUID),
		},
		Spec: creator_v1.ServiceSpec{
			BusinessUnit:       data.BusinessUnit,
			ResourceOwner:      data.ServiceOwner.Username,
			SSAMContainerName:  data.SSAMContainerName,
			LoggingID:          data.LoggingID,
			PagerDutyServiceID: data.PagerDutyServiceID,
			Metadata:           creator_v1.ServiceMetadata{},
			ResourceTags:       parsePlatformTags(data.Tags),
		},
	}

	if data.Compliance != nil {
		service.Status.Compliance = creator_v1.Compliance{
			PRGBControl: data.Compliance.PRGBControl,
		}
	}

	pagerDutyMetadata, err := GetPagerDutyMetadata(&data.ServiceDataWrite)
	if err != nil {
		return nil, err
	}
	if pagerDutyMetadata != nil {
		service.Spec.Metadata.PagerDuty = pagerDutyMetadata
	}

	bambooMetadata, err := GetBambooMetadata(&data.ServiceDataWrite)
	if err != nil {
		return nil, err
	}
	if bambooMetadata != nil {
		service.Spec.Metadata.Bamboo = bambooMetadata
	}

	if data.CreationTimestamp != nil {
		creationTimestamp, err := time.Parse(time.RFC3339, *data.CreationTimestamp)
		if err != nil {
			return nil, err
		}
		service.CreationTimestamp = meta_v1.NewTime(creationTimestamp)
	}
	return service, nil
}

func GetMiscData(data *ServiceDataWrite, key string) (string, error) {
	for _, miscData := range data.Misc {
		if miscData.Key == key {
			return miscData.Value, nil
		}
	}
	return "", nil
}

func SetMiscData(data *ServiceDataWrite, key string, value string) error {
	result := make([]miscData, 0, len(data.Misc))
	for _, miscData := range data.Misc {
		if miscData.Key == key {
			continue // skip conflicting blob with the same name
		}
		result = append(result, miscData)
	}

	md := miscData{
		Key:   key,
		Value: value,
	}
	result = append(result, md)
	data.Misc = result
	return nil
}
