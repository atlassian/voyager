package reporterreporter

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTransientError(t *testing.T) {
	t.Parallel()

	assert.True(t, isRetriableError(http.StatusRequestTimeout))
	assert.True(t, isRetriableError(500))
	assert.True(t, isRetriableError(550))
	assert.True(t, isRetriableError(599))

	assert.False(t, isRetriableError(499))
	assert.False(t, isRetriableError(600))
}
