package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

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
