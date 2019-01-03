package ssam

import (
	"context"
	"fmt"
	"testing"

	"github.com/atlassian/voyager/pkg/util/httputil"
	"github.com/atlassian/voyager/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SSAMClientStub struct {
	containers []*Container
}

func (c SSAMClientStub) GetInternalStore() []*Container {
	return c.containers
}

func (c SSAMClientStub) GetContainer(ctx context.Context, containerName string) (*Container, error) {
	for _, container := range c.containers {
		if container.ShortName == containerName {
			return container, nil
		}
	}
	return nil, httputil.NewNotFound("SSAM container not found")
}

func (c *SSAMClientStub) PostContainer(ctx context.Context, containerRequest *ContainerPostRequest) (*Container, error) {
	for _, container := range c.containers {
		if container.ShortName == containerRequest.ShortName {
			return nil, httputil.NewConflict("Container already exists with that name")
		}
	}

	container := &Container{
		ContainerType: containerRequest.ContainerType,
		DisplayName:   containerRequest.DisplayName,
		ShortName:     containerRequest.ShortName,
		URL:           containerRequest.URL,
		SystemOwner:   containerRequest.SystemOwner,
		Delegates:     containerRequest.Delegates,
	}
	c.containers = append(c.containers, container)
	return container, nil
}

func (c *SSAMClientStub) GetContainerAccessLevel(ctx context.Context, containerName, accessLevelName string) (*AccessLevel, error) {
	container, err := c.GetContainer(ctx, containerName)
	if err != nil {
		return nil, err
	}
	for _, accessLevel := range container.AccessLevels {
		if accessLevel.ShortName == accessLevelName {
			return accessLevel, nil
		}
	}
	return nil, httputil.NewNotFound("SSAM access level was not found")
}

func (c *SSAMClientStub) PostAccessLevel(ctx context.Context, containerName string, accessLevelRequest *AccessLevelPostRequest) (*AccessLevel, error) {
	container, err := c.GetContainer(ctx, containerName)
	if err != nil {
		return nil, err
	}

	accessLevel, err := c.GetContainerAccessLevel(ctx, containerName, accessLevelRequest.ShortName)
	if accessLevel != nil {
		return nil, httputil.NewConflict("Container already has an access level with that name")
	}

	accessLevel = &AccessLevel{
		Name:        accessLevelRequest.Name,
		ShortName:   accessLevelRequest.ShortName,
		Members:     accessLevelRequest.Members,
		System:      container.ShortName,
		ADGroupName: fmt.Sprintf("%s-dl-%s", container.ShortName, accessLevelRequest.ShortName),
	}
	container.AccessLevels = append(container.AccessLevels, accessLevel)
	return accessLevel, nil
}

func (c *SSAMClientStub) findContainer(containerShortName string) (int, error) {
	for i, container := range c.containers {
		if container.ShortName == containerShortName {
			return i, nil
		}
	}
	return 0, httputil.NewNotFound("Container was not found")
}

func (c *SSAMClientStub) DeleteContainer(ctx context.Context, containerShortName string) error {
	index, err := c.findContainer(containerShortName)
	if err != nil {
		return err
	}

	// We don't care about ordering, so move the end to the index and cut the end off.
	c.containers[index] = c.containers[len(c.containers)-1]
	c.containers[len(c.containers)-1] = nil // Apparently, there are memory leaks without this statement.
	c.containers = c.containers[:len(c.containers)-1]

	return nil
}

func NewSSAMClientStub() Client {
	return &SSAMClientStub{}
}

func TestCreateService(t *testing.T) {
	t.Parallel()

	client := NewSSAMClientStub()
	creator := NewServiceCreator(client)

	_, _, err := creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie",
	})

	require.NoError(t, err)
	containers := client.(*SSAMClientStub).GetInternalStore()
	assert.Len(t, containers, 1)

	container := containers[0]
	assert.Len(t, container.AccessLevels, 4)
	elements := make(map[string]string)
	for _, al := range container.AccessLevels {
		elements[al.ShortName] = al.ADGroupName
	}
	require.Equal(t, elements["dev"], "paas-charlies-chaotic-camels-dl-dev")
	require.Equal(t, elements["staging"], "paas-charlies-chaotic-camels-dl-staging")
	require.Equal(t, elements["prod"], "paas-charlies-chaotic-camels-dl-prod")
	require.Equal(t, elements["admins"], "paas-charlies-chaotic-camels-dl-admins")
}

func TestCreateServiceWithDifferentOwner(t *testing.T) {
	t.Parallel()

	client := NewSSAMClientStub()
	creator := NewServiceCreator(client)

	_, _, err := creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie-alpha",
	})

	require.NoError(t, err)

	_, _, err = creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie-beta",
	})

	require.Error(t, err)
}

func TestCreateServiceContainerWithMissingAccessLevels(t *testing.T) {
	t.Parallel()

	const service = "charlies-chaotic-camels"
	const owner = "charlie-alpha"
	const containerName = "paas-" + service
	client := &SSAMClientStub{
		containers: []*Container{
			{
				SystemOwner: owner,
				ShortName:   containerName,
				AccessLevels: []*AccessLevel{
					{
						ShortName: "existing-access-level", // Existing container with a wrong access level
					},
				},
			},
		},
	}
	creator := NewServiceCreator(client)

	_, _, err := creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  service,
		ServiceOwner: owner,
	})

	require.NoError(t, err)

	container, err := client.GetContainer(testutil.ContextWithLogger(t), containerName)
	require.NoError(t, err)
	require.NotNil(t, container)
	assert.Len(t, container.AccessLevels, 5)

	expectedAccessLevels := map[string]struct{}{
		"existing-access-level": {}, // Existing access level should be left untouched
		"dev":                   {},
		"staging":               {},
		"prod":                  {},
		"admins":                {},
	}
	actualAccessLevels := make(map[string]struct{}, 4)
	for _, accessLevel := range container.AccessLevels {
		actualAccessLevels[accessLevel.ShortName] = struct{}{}
	}
	assert.Equal(t, expectedAccessLevels, actualAccessLevels)
}

func TestCreateServiceServiceNameWithInvalidNameCausesError(t *testing.T) {
	t.Parallel()

	client := NewSSAMClientStub()
	creator := NewServiceCreator(client)

	_, _, err := creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		// we're just going by a name we know is bad
		ServiceName:  "known-bad-dl-name",
		ServiceOwner: "charlie-the-service-owner",
	})

	require.Error(t, err)
}

func TestCreateServiceThenDeleteService(t *testing.T) {
	t.Parallel()

	// Given
	client := NewSSAMClientStub()
	creator := NewServiceCreator(client)

	// When
	_, accessLevels, err := creator.CreateService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie",
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, "paas-charlies-chaotic-camels-dl-prod", accessLevels.Production)
	containers := client.(*SSAMClientStub).GetInternalStore()
	assert.Len(t, containers, 1)

	container := containers[0]
	assert.Len(t, container.AccessLevels, 4)
	elements := make(map[string]string)
	for _, al := range container.AccessLevels {
		elements[al.ShortName] = al.ADGroupName
	}
	require.Equal(t, elements["dev"], "paas-charlies-chaotic-camels-dl-dev")
	require.Equal(t, elements["staging"], "paas-charlies-chaotic-camels-dl-staging")
	require.Equal(t, elements["prod"], "paas-charlies-chaotic-camels-dl-prod")
	require.Equal(t, elements["admins"], "paas-charlies-chaotic-camels-dl-admins")

	// When
	deleteErr := creator.DeleteService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie",
	})

	// Then
	require.NoError(t, deleteErr)

	// When
	doubleDeleteErr := creator.DeleteService(testutil.ContextWithLogger(t), &ServiceMetadata{
		ServiceName:  "charlies-chaotic-camels",
		ServiceOwner: "charlie",
	})

	require.NoError(t, doubleDeleteErr)
}
