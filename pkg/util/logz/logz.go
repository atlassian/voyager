package logz

import (
	"context"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type loggerContextKeyType uint64

const LoggerContextKey loggerContextKeyType = 2718281828459045235

func CreateContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, LoggerContextKey, logger)
}

func RetrieveLoggerFromContext(ctx context.Context) *zap.Logger {
	if log, ok := ctx.Value(LoggerContextKey).(*zap.Logger); ok && log != nil {
		return log
	}

	panic(errors.New("context did not contain logger, please call CreateContextWithLogger"))
}

// Sync is useful for when you want to use defer logger.Sync and do not want to handle errors.
func Sync(logger *zap.Logger) {
	_ = logger.Sync() // nolint
}
