// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package health

import (
	"context"
	"errors"
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

func TestWaitReadyTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "never ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := WaitReady(context.Background(), Options{
		URL:            server.URL,
		Timeout:        50 * time.Millisecond,
		PollInterval:   10 * time.Millisecond,
		RequestTimeout: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("expected last HTTP status in error, got %q", err.Error())
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
