package logz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type testContextKey string

func TestCreateContextWithLogger(t *testing.T) {
	t.Parallel()

	mySpecialKey := testContextKey("7BjFpdIKdPq9gP76b4QMoCj0yvw1kYmd")
	mySpecialValue := "19YO5pduL5YLq5jDXsbDtw50Sw4e8F9T"
	ctx := context.WithValue(context.Background(), mySpecialKey, mySpecialValue)

	ctxLogger := CreateContextWithLogger(ctx, nil)

	assert.NotNil(t, ctxLogger.Value(mySpecialKey), "value was not inherited from parent correctly: did not exist")
	fromContext, ok := ctxLogger.Value(mySpecialKey).(string)
	assert.True(t, ok, "value was not inherited from parent correctly: it was not a string")
	assert.Equal(t, fromContext, mySpecialValue, "value was not inherited from parent correctly: different value")

}

func TestCreateContextThenRetrieve(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	ctxLogger := CreateContextWithLogger(context.Background(), logger)

	assert.NotPanics(t, func() {
		RetrieveLoggerFromContext(ctxLogger)
	})

	loggerFromContext := RetrieveLoggerFromContext(ctxLogger)
	assert.Equal(t, loggerFromContext, logger, "The logger we gave the parent, was not the same in the child")
}

func TestRetrievePanicsOnBadContext(t *testing.T) {
	t.Parallel()

	ctxLogger := context.Background()

	assert.Panics(t, func() {
		RetrieveLoggerFromContext(ctxLogger)
	})
}
