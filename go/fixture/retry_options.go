// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"
)

const requestAlreadyInFlightMarker = "REQUEST_ALREADY_IN_FLIGHT"

func isTransientStatus(e *httpStatusError) bool {
	switch e.statusCode {
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusConflict:
		return bytes.Contains(e.body, []byte(requestAlreadyInFlightMarker))
	}
	return false
}

// retryOptions is a bounded exponential-backoff policy for retrying a
// transient failure while the participant is still warming up.
type retryOptions struct {
	// MaxAttempts is the total number of attempts, including the first.
	MaxAttempts int
	// BaseDelay is the delay before the first retry; it doubles on each
	// subsequent retry.
	BaseDelay time.Duration
	// MaxDelay caps every computed backoff delay.
	MaxDelay time.Duration
	// Sleep waits between attempts and returns early with the context's
	// error when ctx is done. Tests inject a no-op so they never sleep.
	Sleep func(ctx context.Context, d time.Duration) error
}

// defaultRetryOptions returns the production policy: 6 attempts, a 1s base
// delay, a 16s cap and a context-aware sleep.
func defaultRetryOptions() retryOptions {
	return retryOptions{
		MaxAttempts: 6,
		BaseDelay:   time.Second,
		MaxDelay:    16 * time.Second,
		Sleep:       sleepContext,
	}
}

func (r retryOptions) validate() error {
	switch {
	case r.MaxAttempts < 1:
		return errors.New("retry: MaxAttempts must be at least 1")
	case r.BaseDelay < 0:
		return errors.New("retry: BaseDelay must not be negative")
	case r.MaxDelay < r.BaseDelay:
		return errors.New("retry: MaxDelay must not be below BaseDelay")
	case r.Sleep == nil:
		return errors.New("retry: Sleep is required")
	}
	return nil
}

func (r retryOptions) delayForAttempt(attempt int) time.Duration {
	delay := r.BaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= r.MaxDelay {
			return r.MaxDelay
		}
	}
	if delay > r.MaxDelay {
		return r.MaxDelay
	}
	return delay
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
