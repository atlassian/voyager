package reporterreporter

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTransientError(t *testing.T) {
	t.Parallel()

	assert.True(t, isRetriableError(http.StatusRequestTimeout))
	assert.True(t, isRetriableError(http.StatusTooManyRequests))
	assert.True(t, isRetriableError(http.StatusServiceUnavailable))
	assert.True(t, isRetriableError(http.StatusGatewayTimeout))

	assert.False(t, isRetriableError(500))
	assert.False(t, isRetriableError(501))
	assert.False(t, isRetriableError(600))
	assert.False(t, isRetriableError(400))
}
