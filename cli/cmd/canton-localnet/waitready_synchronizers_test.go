// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/health"
	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
)

func TestWaitForSynchronizersPresentsAFreshlyMintedTokenOnEveryPoll(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	mints := 0
	var authorizations []string
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		mints++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":300}`, mints)
	}))
	defer tokens.Close()
	ledger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if len(authorizations) < 3 {
			_, _ = w.Write([]byte(`{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::1"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::1"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app::1"}]}`))
	}))
	defer ledger.Close()
	ep := slot.Endpoints{
		Slot:             slot.Slot{Canonical: "a-validator-1", AuthKind: slot.AuthKindOAuth2},
		TokenURLHost:     tokens.URL,
		ClientID:         "client",
		ClientSecret:     "secret",
		JSONLedgerAPIURL: ledger.URL,
	}
	opts := health.Options{Timeout: 30 * time.Second, PollInterval: time.Millisecond}
	var logs []string
	log := func(msg string) { logs = append(logs, msg) }

	err := waitForSynchronizers(context.Background(), ep, 2, opts, http.DefaultClient, log)
	if err != nil {
		t.Fatalf("waitForSynchronizers: %v (logs: %s)", err, strings.Join(logs, "; "))
	}

	want := "Bearer token-1,Bearer token-2,Bearer token-3"
	if got := strings.Join(authorizations, ","); got != want {
		t.Errorf("expected one fresh token per poll %q, got %q", want, got)
	}
}

func TestWaitForSynchronizersFailsAtOnceWhenTheFirstTokenCannotBeMinted(t *testing.T) {
	t.Parallel()
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tokens.Close()
	ep := slot.Endpoints{
		Slot:         slot.Slot{Canonical: "a-validator-1", AuthKind: slot.AuthKindOAuth2},
		TokenURLHost: tokens.URL,
		ClientID:     "client",
		ClientSecret: "secret",
	}
	opts := health.Options{Timeout: 30 * time.Second, PollInterval: time.Millisecond}

	err := waitForSynchronizers(context.Background(), ep, 2, opts, http.DefaultClient, func(string) {})

	if err == nil || !strings.Contains(err.Error(), "wait-ready: mint token:") {
		t.Fatalf("expected an immediate mint-token failure, got %v", err)
	}
}

func TestWaitForSynchronizersRetriesWhenALaterTokenMintFails(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	mints := 0
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		mints++
		if mints == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokens.Close()
	polls := 0
	ledger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		polls++
		w.Header().Set("Content-Type", "application/json")
		if polls < 2 {
			_, _ = w.Write([]byte(`{"connectedSynchronizers":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"connectedSynchronizers":[{"synchronizerAlias":"a","synchronizerId":"a::1"},{"synchronizerAlias":"b","synchronizerId":"b::1"}]}`))
	}))
	defer ledger.Close()
	ep := slot.Endpoints{
		Slot:             slot.Slot{Canonical: "a-validator-1", AuthKind: slot.AuthKindOAuth2},
		TokenURLHost:     tokens.URL,
		ClientID:         "client",
		ClientSecret:     "secret",
		JSONLedgerAPIURL: ledger.URL,
	}
	opts := health.Options{Timeout: 30 * time.Second, PollInterval: time.Millisecond}

	err := waitForSynchronizers(context.Background(), ep, 2, opts, http.DefaultClient, func(string) {})

	if err != nil {
		t.Fatalf("a transient mint failure after the first poll must not abort the wait: %v", err)
	}
}
