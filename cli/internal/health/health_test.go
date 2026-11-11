// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package health

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitReadySuccessAfterRetries(t *testing.T) {
	t.Parallel()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count < 3 {
			http.Error(w, "not yet", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := WaitReady(context.Background(), Options{
		URL:            server.URL,
		Timeout:        2 * time.Second,
		PollInterval:   10 * time.Millisecond,
		RequestTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Errorf("expected at least 3 attempts, got %d", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func clientAnswering(status int) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})}
}

func clientFailingOnceThenBlocking(firstErr error) *http.Client {
	var calls int32
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, firstErr
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
}

func clientAnsweringOnceThenBlocking(status int) *http.Client {
	var calls int32
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
}

func TestWaitReadyTimeout(t *testing.T) {
	t.Parallel()

	err := WaitReady(context.Background(), Options{
		URL:            "http://localnet.invalid/readyz",
		Timeout:        50 * time.Millisecond,
		PollInterval:   10 * time.Millisecond,
		RequestTimeout: 20 * time.Millisecond,
		HTTPClient:     clientAnswering(http.StatusServiceUnavailable),
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "last status: HTTP 503") {
		t.Errorf("expected last HTTP status in error, got %q", err.Error())
	}
}

func TestWaitReadyTimeoutKeepsTheLastStatusWhenTheFinalAttemptIsCutByTheOverallDeadline(t *testing.T) {
	t.Parallel()

	err := WaitReady(context.Background(), Options{
		URL:            "http://localnet.invalid/readyz",
		Timeout:        100 * time.Millisecond,
		PollInterval:   time.Millisecond,
		RequestTimeout: time.Minute,
		HTTPClient:     clientAnsweringOnceThenBlocking(http.StatusServiceUnavailable),
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "last status: HTTP 503") {
		t.Errorf("expected the kept HTTP 503 in error, got %q", err.Error())
	}
}

func TestWaitReadyTimeoutKeepsTheLastRealErrorWhenTheFinalAttemptIsCutByTheOverallDeadline(t *testing.T) {
	t.Parallel()

	err := WaitReady(context.Background(), Options{
		URL:            "http://localnet.invalid/readyz",
		Timeout:        100 * time.Millisecond,
		PollInterval:   time.Millisecond,
		RequestTimeout: time.Minute,
		HTTPClient:     clientFailingOnceThenBlocking(errors.New("connection refused")),
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected the kept connection error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("deadline-cut attempt overwrote the last real error: %q", err.Error())
	}
}

func TestWaitReadyTimeoutReportsTheDeadlineCutAttemptWhenNothingElseWasObserved(t *testing.T) {
	t.Parallel()

	err := WaitReady(context.Background(), Options{
		URL:            "http://localnet.invalid/readyz",
		Timeout:        30 * time.Millisecond,
		PollInterval:   time.Millisecond,
		RequestTimeout: time.Minute,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		})},
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "last error: Get \"http://localnet.invalid/readyz\": context deadline exceeded") {
		t.Errorf("expected the deadline-cut error when it is the only one, got %q", err.Error())
	}
}

func TestWaitReadyTimeoutKeepsThePerRequestTimeoutAsTheLastError(t *testing.T) {
	t.Parallel()
	var calls int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, errors.New("connection refused")
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}

	err := WaitReady(context.Background(), Options{
		URL:            "http://localnet.invalid/readyz",
		Timeout:        150 * time.Millisecond,
		PollInterval:   time.Millisecond,
		RequestTimeout: 10 * time.Millisecond,
		HTTPClient:     client,
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected the per-request timeout to count as the last error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "connection refused") {
		t.Errorf("per-request timeout did not replace the earlier error: %q", err.Error())
	}
}

func TestWaitReadyTimeoutSurfacesNetworkError(t *testing.T) {
	t.Parallel()
	err := WaitReady(context.Background(), Options{
		URL:            "http://127.0.0.1:1/readyz",
		Timeout:        30 * time.Millisecond,
		PollInterval:   10 * time.Millisecond,
		RequestTimeout: 10 * time.Millisecond,
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "last error") {
		t.Errorf("expected last error in message, got %q", err.Error())
	}
}

func TestWaitReadyHonoursRequestTimeoutOnCustomClient(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	clientWithoutTimeout := &http.Client{}

	start := time.Now()
	err := WaitReady(context.Background(), Options{
		URL:            server.URL,
		Timeout:        200 * time.Millisecond,
		PollInterval:   20 * time.Millisecond,
		RequestTimeout: 30 * time.Millisecond,
		HTTPClient:     clientWithoutTimeout,
	})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("RequestTimeout not enforced on caller-supplied client: total elapsed %v exceeds overall budget", elapsed)
	}
}

func TestWaitReadyHonoursContextCancel(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "never", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := WaitReady(ctx, Options{
		URL:            server.URL,
		Timeout:        5 * time.Second,
		PollInterval:   5 * time.Millisecond,
		RequestTimeout: 10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if errors.Is(err, ErrTimeout) {
		t.Fatalf("expected context cancellation error, got ErrTimeout")
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	if opts.URL != DefaultURL {
		t.Errorf("expected URL %q, got %q", DefaultURL, opts.URL)
	}
	if opts.Timeout != 5*time.Minute {
		t.Errorf("expected 5m timeout, got %v", opts.Timeout)
	}
}

func TestWaitReadyReturnsTheAbortErrorWithoutWaitingForTheTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "never ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var aborts int32
	fatal := errors.New("splice container is crash-looping")

	err := WaitReady(context.Background(), Options{
		URL:            server.URL,
		Timeout:        time.Minute,
		PollInterval:   time.Millisecond,
		RequestTimeout: 500 * time.Millisecond,
		Abort: func(context.Context) error {
			if atomic.AddInt32(&aborts, 1) == 3 {
				return fatal
			}
			return nil
		},
	})

	if !errors.Is(err, fatal) {
		t.Fatalf("want the abort error, got %v", err)
	}
	if got := atomic.LoadInt32(&aborts); got != 3 {
		t.Fatalf("want 3 abort checks, got %d", got)
	}
}

func TestWaitReadyDoesNotConsultAbortOnceReady(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := WaitReady(context.Background(), Options{
		URL:     server.URL,
		Timeout: time.Second,
		Abort:   func(context.Context) error { return errors.New("must not be called") },
	})

	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
