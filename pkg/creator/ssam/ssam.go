package ssam

import (
	"context"
	"fmt"
	"strings"

	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/creator/ssam/util/zappers"
	"github.com/atlassian/voyager/pkg/ssam"
	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// 'paas' is an SSAM prefix owned by Voyager
	defaultSSAMPrefix    = "paas"
	defaultContainerType = "paas"

	adminsAccessLevelShortName = "admins"
	adminsAccessLevelName      = "Micros Admins"
)

var (
	envTypes = []voyager.EnvType{
		voyager.EnvTypeDev,
		voyager.EnvTypeStaging,
		voyager.EnvTypeProduction,
	}
)

func stringSliceContains(arr []string, target string) bool {
	for _, value := range arr {
		if value == target {
			return true
		}
	}
	return false
}

type ServiceMetadata struct {
	ServiceName  voyager.ServiceName
	ServiceOwner string
}

type AccessLevels struct {
	Development string
	Staging     string
	Production  string
	Admins      string
}

// SSAMContainerShortName creates the short-name that Voyager uses for Containers. It's for computers. It is also known
// as the Container Name.
func (m *ServiceMetadata) SSAMContainerShortName() string {
	return fmt.Sprintf("%s-%s", defaultSSAMPrefix, m.ServiceName)
}

// SSAMContainerDisplayName creates the display name for humans to read. It's for humans.
func (m *ServiceMetadata) SSAMContainerDisplayName() string {
	return fmt.Sprintf("%s Service", m.ServiceName)
}

// SSAMAccessLevelShortName returns short-name of an access level; it's for computers.
func (m *ServiceMetadata) SSAMAccessLevelShortName(envType voyager.EnvType) string {
	return string(envType)
}

// SSAMAccessLevelName returns the display name for an access level; it's for humans.
func (m *ServiceMetadata) SSAMAccessLevelName(envType voyager.EnvType) string {
	return fmt.Sprintf("Micros 2 %s", envType)
}

// matchesContainer checks whether the container matches the metadata
func (m *ServiceMetadata) matchesContainer(container *Container) error {
	if container.ShortName != m.SSAMContainerShortName() {
		return errors.Errorf("container short name does not match: request short name = %q; existing short name = %q",
			m.SSAMContainerShortName(), container.ShortName)
	}

	if container.SystemOwner != m.ServiceOwner {
		return errors.Errorf("container owner is someone else: requested service owner = %q; existing service owner = %q",
			m.ServiceOwner, container.SystemOwner)
	}

	return nil
}

func (m *ServiceMetadata) validate() error {
	return ssam.ValidateServiceName(m.ServiceName)
}

type ServiceCreator struct {
	client Client
}

func NewServiceCreator(client Client) *ServiceCreator {
	return &ServiceCreator{
		client: client,
	}
}

func (s *ServiceCreator) GetExpectedServiceContainerName(ctx context.Context, metadata *ServiceMetadata) string {
	return metadata.SSAMContainerShortName()
}

// CreateService creates a "service" which is a container and access levels for that container.
// The short name for the container is returned.
//
// The container is not created willy-nilly. Voyager has more authorisation than the user we are acting as a proxy for.
// We need to check if we can create the container, or use a pre-existing one that has the data we are looking for. We
// also ensure that the default access levels exist.
func (s *ServiceCreator) CreateService(ctx context.Context, metadata *ServiceMetadata) (string, AccessLevels, error) {
	logger := logz.RetrieveLoggerFromContext(ctx).With(
		logz.ServiceName(metadata.ServiceName),
		zappers.ServiceOwner(metadata.ServiceOwner),
		zappers.ContainerShortName(metadata.SSAMContainerShortName()))
	logger.Info("Creating SSAM Container and Access Level")

	if err := metadata.validate(); err != nil {
		return "", AccessLevels{}, err
	}

	if _, err := s.createOrUseExistingContainer(ctx, logger, metadata); err != nil {
		return "", AccessLevels{}, errors.Wrapf(err, "failed to create container %q", metadata.SSAMContainerShortName())
	}

	accessLevels := AccessLevels{}

	for _, envType := range envTypes {
		accessLevel, err := s.createEnvAccessLevel(ctx, logger, metadata, envType)
		if err != nil {
			return "", AccessLevels{}, errors.Wrapf(err, "failed to create access levels for SSAM container %q", metadata.SSAMContainerShortName())
		}
		switch envType {
		case voyager.EnvTypeDev:
			accessLevels.Development = accessLevel.ADGroupName
		case voyager.EnvTypeStaging:
			accessLevels.Staging = accessLevel.ADGroupName
		case voyager.EnvTypeProduction:
			accessLevels.Production = accessLevel.ADGroupName
		}
	}

	adminsAccessLevel, err := s.createAdminsAccessLevel(ctx, logger, metadata)

	if err != nil {
		return "", AccessLevels{}, errors.Wrapf(err, "failed to create access levels for SSAM container %q", metadata.SSAMContainerShortName())
	}

	accessLevels.Admins = adminsAccessLevel.ADGroupName

	container, err := s.client.GetContainer(ctx, metadata.SSAMContainerShortName())
	if err != nil {
		return "", AccessLevels{}, errors.Wrapf(err, "failed to get container %q that was just created", metadata.SSAMContainerShortName())
	}

	return container.ShortName, accessLevels, nil
}

func (s *ServiceCreator) DeleteService(ctx context.Context, metadata *ServiceMetadata) error {
	logger := logz.RetrieveLoggerFromContext(ctx).With(
		logz.ServiceName(metadata.ServiceName),
		zappers.ServiceOwner(metadata.ServiceOwner),
		zappers.ContainerShortName(metadata.SSAMContainerShortName()))
	logger.Info("Deleting SSAM Container and Access Level")

	// Validate the callers's input to our function, we don't want SHIT getting into our function. Our pretty, pretty
	// function.
	if err := metadata.validate(); err != nil {
		return err
	}

	// Delete the requested container from SSAM. This should delete all of the access levels that were associated with
	// that container. Therefore we don't need to make N network requests to delete N access levels.
	if err := s.deleteContainer(ctx, logger, metadata); err != nil {
		return errors.Wrapf(err, "failed to delete container %q", metadata.SSAMContainerShortName())
	}

	// Get the container and see if it exists, it shouldn't.
	_, err := s.client.GetContainer(ctx, metadata.SSAMContainerShortName())
	if err == nil {
		return errors.Wrapf(
			err, "failed to delete container %q, after attempting to delete the container still exists", metadata.SSAMContainerShortName())
	} else if err != nil && !httputil.IsNotFound(err) {
		return errors.Wrapf(err, "failed to confirm the container %q was deleted", metadata.SSAMContainerShortName())
	}

	return nil
}

func (s *ServiceCreator) createEnvAccessLevel(ctx context.Context, logger *zap.Logger, metadata *ServiceMetadata, envType voyager.EnvType) (*AccessLevel, error) {
	return s.createSSAMAccessLevel(ctx, logger, metadata.ServiceOwner, metadata.SSAMContainerShortName(), metadata.SSAMAccessLevelName(envType), metadata.SSAMAccessLevelShortName(envType))
}

func (s *ServiceCreator) createAdminsAccessLevel(ctx context.Context, logger *zap.Logger, metadata *ServiceMetadata) (*AccessLevel, error) {
	return s.createSSAMAccessLevel(ctx, logger, metadata.ServiceOwner, metadata.SSAMContainerShortName(), adminsAccessLevelName, adminsAccessLevelShortName)
}

func (s *ServiceCreator) createSSAMAccessLevel(ctx context.Context, logger *zap.Logger, serviceOwner, containerShortName, accessLevelName, accessLevelShortName string) (*AccessLevel, error) {

	accessLevel, err := s.client.GetContainerAccessLevel(ctx, containerShortName, accessLevelShortName)

	if err != nil && !httputil.IsNotFound(err) {
		return nil, errors.Wrapf(err, "could not get access level %q for container %q", accessLevelShortName, containerShortName)
	}

	if accessLevel != nil {
		if accessLevel.ShortName != accessLevelShortName {
			return nil, errors.Errorf(
				"expected access level %q on container %q from SSAM, but access level %q was incorrectly returned",
				accessLevelShortName, containerShortName, accessLevel.ShortName)
		}

		if !stringSliceContains(accessLevel.Members.Users, serviceOwner) {
			return nil, errors.Errorf(
				"existing container %q has existing access level %q that does not have requester (%q) listed as a member, current members are %q",
				containerShortName,
				accessLevelShortName,
				serviceOwner,
				strings.Join(accessLevel.Members.Users, ", "))
		}

		logger.Info("Using existing SSAM Access Level for service")
		return accessLevel, nil
	}

	newAccessLevel, err := s.client.PostAccessLevel(ctx, containerShortName, &AccessLevelPostRequest{
		Name:      accessLevelName,
		ShortName: accessLevelShortName,
		Members: &AccessLevelMembers{
			Users: []string{serviceOwner},
		},
	})

	if err != nil {
		return nil, errors.Wrapf(err, "request to SSAM to create access level %q for container %q failed",
			accessLevelShortName, containerShortName)
	}

	if newAccessLevel == nil || newAccessLevel.ADGroupName == "" {
		return nil, errors.Errorf("new access level %q for container %q has empty/missing ad_group_name in response",
			accessLevelShortName, containerShortName)
	}

	return newAccessLevel, nil
}

func (s *ServiceCreator) createOrUseExistingContainer(ctx context.Context, logger *zap.Logger, metadata *ServiceMetadata) (*Container, error) {
	container, err := s.client.GetContainer(ctx, metadata.SSAMContainerShortName())

	if err != nil && !httputil.IsNotFound(err) {
		return nil, errors.Wrap(err, "failed to check if container already exists")
	}

	if container != nil {
		if matchErr := metadata.matchesContainer(container); matchErr != nil {
			return nil, errors.Wrapf(matchErr, "a container already exists but can not be used")
		}
		logger.Info("Using existing SSAM container for service", zappers.ContainerShortName(metadata.SSAMContainerShortName()))
		return container, nil
	}

	container, err = s.client.PostContainer(ctx, &ContainerPostRequest{
		SystemOwner:   metadata.ServiceOwner,
		DisplayName:   metadata.SSAMContainerDisplayName(),
		ShortName:     metadata.SSAMContainerShortName(),
		ContainerType: defaultContainerType,
	})

	if err != nil {
		return nil, errors.Wrapf(err, "request to SSAM to create container failed")
	}

	logger.Info("Created new SSAM container for service")
	return container, nil
}

func (s *ServiceCreator) deleteContainer(ctx context.Context, logger *zap.Logger, metadata *ServiceMetadata) error {
	container, err := s.client.GetContainer(ctx, metadata.SSAMContainerShortName())

	if httputil.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "failed to check if container already exists")
	}

	if matchErr := metadata.matchesContainer(container); matchErr != nil {
		return errors.Wrapf(matchErr, "a container with the same short name exists but does not fully match the container that was requested to be deleted")
	}

	err = s.client.DeleteContainer(ctx, metadata.SSAMContainerShortName())

	if err != nil && !httputil.IsNotFound(err) {
		return errors.Wrapf(err, "request to SSAM to delete container failed")
	}

	logger.Info("Deleted SSAM container for service")
	return nil
}
