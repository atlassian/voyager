package testutil

import (
	"context"
	"testing"

	"github.com/atlassian/voyager/pkg/util/logz"
	"go.uber.org/zap/zaptest"
)

func ContextWithLogger(t *testing.T) context.Context {
	return logz.CreateContextWithLogger(context.Background(), zaptest.NewLogger(t))
}
