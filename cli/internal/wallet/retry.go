// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package wallet

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultBudget   = 60 * time.Second
	initialBackoff  = time.Second
	maxBackoff      = 8 * time.Second
	backoffMultiple = 2
)

// Retry bounds how long a wallet call keeps retrying transient answers
// (HTTP 429, 502, 503, 504 and connection refused), which a validator app
// gives while it is still settling after boot. The non-idempotent tap
// retries only 429 and connection refused, the answers that guarantee the
// request was not processed. Budget is the total time
// spent waiting between attempts; zero means 60 seconds. Sleep waits for
// a delay and returns early with the context's error; nil sleeps for real.
type Retry struct {
	Budget time.Duration
	Sleep  func(ctx context.Context, delay time.Duration) error
}

type answer struct {
	status     int
	body       []byte
	retryAfter time.Duration
}

func (r Retry) run(ctx context.Context, retryable func(status int, err error) bool, send func() (answer, error)) (answer, error) {
	budget := r.Budget
	if budget == 0 {
		budget = defaultBudget
	}
	sleep := r.Sleep
	if sleep == nil {
		sleep = sleepFor
	}
	var waited time.Duration
	backoff := initialBackoff
	for {
		got, err := send()
		if !retryable(got.status, err) {
			return got, err
		}
		delay := backoff
		if got.retryAfter > 0 {
			delay = got.retryAfter
		}
		if waited+delay > budget {
			return got, err
		}
		if sleepErr := sleep(ctx, delay); sleepErr != nil {
			return got, sleepErr
		}
		waited += delay
		backoff = min(backoff*backoffMultiple, maxBackoff)
	}
}

func transientAnswer(status int, err error) bool {
	if err != nil {
		return errors.Is(err, syscall.ECONNREFUSED)
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func requestNeverProcessed(status int, err error) bool {
	if err != nil {
		return errors.Is(err, syscall.ECONNREFUSED)
	}
	return status == http.StatusTooManyRequests
}

func sleepFor(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readAnswer(resp *http.Response) (answer, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	seconds, parseErr := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	got := answer{status: resp.StatusCode, body: body}
	if parseErr == nil && seconds > 0 {
		got.retryAfter = time.Duration(seconds) * time.Second
	}
	return got, err
}
