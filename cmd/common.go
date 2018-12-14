package cmd

import (
	"context"
	"os"

	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/pkg/errors"
)

// ExitOnError will log the error and exit if the error is not due to the context being cancelled
func ExitOnError(err error) {
	if err != nil && errors.Cause(err) != context.Canceled {
		crash.LogErrorAsJSON(err)
		os.Exit(1)
	}
}

func RunInterruptably(fn func(context.Context) error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	defer crash.LogPanicAsJSON()
	ctrlApp.CancelOnInterrupt(ctx, cancelFunc)

	ExitOnError(fn(ctx))
}
