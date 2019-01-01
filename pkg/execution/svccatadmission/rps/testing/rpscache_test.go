package testing

import (
	"context"
	"testing"

	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Making sure that the actual type implements the interface
var _ rps.Client = &rps.ClientImpl{}

func TestGetServiceForExistingThing(t *testing.T) {
	t.Parallel()
	rpsCache := MockRPSCache(t)
	serviceName, err := rpsCache.GetServiceFor(context.Background(), "9a3f2d35-0ce8-48b7-8531-a72b5cd02fd4")
	require.NoError(t, err)
	assert.EqualValues(t, "node-refapp-jhaggerty-dev", serviceName)
}

func TestGetServiceForMissingThing(t *testing.T) {
	t.Parallel()
	rpsCache := MockRPSCache(t)
	serviceName, err := rpsCache.GetServiceFor(context.Background(), "something")
	require.NoError(t, err)
	assert.EqualValues(t, "", serviceName)
}
