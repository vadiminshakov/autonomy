package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Exponential executes op with an exponential backoff policy.
//
// The call is retried while shouldRetry(err) == true for returned errors.
// Backoff parameters: initial 1s, multiplier 2, maximum 5 retries.
// The provided context can cancel the retries early.
func Exponential(ctx context.Context, op func() error, shouldRetry func(error) bool) error {
	wrapped := func() error {
		if err := op(); err != nil {
			if shouldRetry != nil && shouldRetry(err) {
				return err
			}

			return backoff.Permanent(err)
		}

		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = time.Second
	bo.Multiplier = 2

	return backoff.Retry(wrapped, backoff.WithContext(backoff.WithMaxRetries(bo, 5), ctx))
}
