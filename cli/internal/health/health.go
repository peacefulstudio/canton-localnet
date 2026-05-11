// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package health probes the JSON Ledger API readiness endpoint with
// bounded retry/backoff. It is the Go equivalent of the
// compose/scripts/wait-ready.sh shell script.
package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultURL is the host-exposed JSON Ledger API readiness endpoint for
// the app-provider participant in the splice LocalNet stack.
const DefaultURL = "http://localhost:3975/readyz"

// Options configures a wait loop. When HTTPClient is supplied, its
// existing timeout is preserved and RequestTimeout is enforced by
// wrapping each request in a per-attempt context deadline so the field
// is always honoured.
type Options struct {
	URL            string
	Timeout        time.Duration
	PollInterval   time.Duration
	RequestTimeout time.Duration
	HTTPClient     *http.Client
	Logger         func(string)
}

// DefaultOptions returns sensible defaults: 5 minute overall timeout,
// 5 second poll interval, 3 second per-request timeout.
func DefaultOptions() Options {
	return Options{
		URL:            DefaultURL,
		Timeout:        5 * time.Minute,
		PollInterval:   5 * time.Second,
		RequestTimeout: 3 * time.Second,
	}
}

// ErrTimeout is returned when WaitReady exhausts its overall timeout
// budget without observing a healthy response.
var ErrTimeout = errors.New("health: timed out waiting for readiness")

// WaitReady polls opts.URL until it returns HTTP 200 or the overall
// timeout elapses. The caller's context can cancel the loop earlier; in
// that case ctx.Err() is returned.
func WaitReady(ctx context.Context, opts Options) error {
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 3 * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.RequestTimeout}
	}
	log := opts.Logger
	if log == nil {
		log = func(string) {}
	}

	deadline := time.Now().Add(opts.Timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var (
		lastStatus int
		lastErr    error
	)
	timedOut := func() error {
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.DeadlineExceeded) {
			return ctxErr
		}
		return formatTimeout(opts.URL, lastStatus, lastErr)
	}

	attempt := 0
	for {
		attempt++
		status, err := probe(ctx, client, opts.URL, opts.RequestTimeout)
		if err == nil && status == http.StatusOK {
			log(fmt.Sprintf("Ready after attempt %d: HTTP %d", attempt, status))
			return nil
		}
		lastStatus, lastErr = status, err
		if err != nil {
			log(fmt.Sprintf("Attempt %d: %s", attempt, err.Error()))
		} else {
			log(fmt.Sprintf("Attempt %d: HTTP %d", attempt, status))
		}

		if time.Now().Add(opts.PollInterval).After(deadline) {
			return timedOut()
		}

		select {
		case <-ctx.Done():
			return timedOut()
		case <-time.After(opts.PollInterval):
		}
	}
}

func formatTimeout(url string, lastStatus int, lastErr error) error {
	switch {
	case lastErr != nil:
		return fmt.Errorf("%w: %s (last error: %v)", ErrTimeout, url, lastErr)
	case lastStatus != 0:
		return fmt.Errorf("%w: %s (last status: HTTP %d)", ErrTimeout, url, lastStatus)
	default:
		return fmt.Errorf("%w: %s", ErrTimeout, url)
	}
}

func probe(ctx context.Context, client *http.Client, url string, requestTimeout time.Duration) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}
