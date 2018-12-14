package util

import (
	"github.com/atlassian/voyager/pkg/k8s/updater"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

// This file provides some Retry utilities that handle retries also including
// checking for WaitTimeout and handling our style of using "retriable" as a
// return value rather than a boolean that indicates "stop retrying".

// Retry unconditionally retries something until it works
func Retry(backoff wait.Backoff, f func() error) error {
	return RetryConditionally(backoff, func() (bool, error) {
		return true, f()
	})
}

// RetryConditionally keeps retrying as long as f() returns true, indicating
// that it is retriable.
func RetryConditionally(retryBackoff wait.Backoff, f func() ( /* retriable */ bool, error)) error {
	var lastError error
	err := wait.ExponentialBackoff(retryBackoff, func() ( /* finished */ bool, error) {
		retriable, err := f()
		if err != nil {
			if retriable {
				lastError = err
				return false, nil
			}
			return false, err
		}

		return true, nil
	})
	if err == wait.ErrWaitTimeout {
		err = lastError
	}
	return err
}

// RetryObjectUpdater retries the objectupdater. It's hardcoded to use the
// retry.DefaultRetry from client-go as it's designed for the conflict scenario
func RetryObjectUpdater(logger *zap.Logger, updater updater.ObjectUpdater, obj runtime.Object) error {
	return RetryConditionally(retry.DefaultRetry, func() (bool, error) {
		conflict, retriable, _, err := updater.CreateOrUpdate(
			logger,
			func(r runtime.Object) error {
				return nil
			},
			obj,
		)

		if conflict {
			// Without this error, the retry could terminate on timeout but return
			// nil, indicating a success, whereas we want it to notify us of a repeated
			// conflict.
			return true, err
		}

		return retriable, err
	})
}
