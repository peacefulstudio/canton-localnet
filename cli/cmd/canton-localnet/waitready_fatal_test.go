// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/startupfatal"
)

func swapStartupFatalRunner(t *testing.T, run startupfatal.Runner) {
	t.Helper()
	original := startupFatalRunner
	startupFatalRunner = run
	t.Cleanup(func() { startupFatalRunner = original })
}

func notReadyServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestWaitReadyAbortsWithTheHintMismatchWhenSpliceCrashLoops(t *testing.T) {
	swapStartupFatalRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "inspect" {
			return []byte("5\n"), nil
		}
		return []byte("PartyId hint localnet-validator-1 does not match configured hint a-validator-1\n"), nil
	})
	server := notReadyServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "30s", "--interval", "1ms")

	if err == nil {
		t.Fatal("want a fatal error, got nil")
	}
	want := "splice container is crash-looping (RestartCount 5): the volume was bootstrapped with party hint localnet-validator-1 but the configured hint is a-validator-1 (slot a-validator-1)."
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("want error containing\n%s\ngot\n%v", want, err)
	}
	if !strings.Contains(err.Error(), "./canton-localnet down --volumes") {
		t.Fatalf("want the remediation, got %v", err)
	}
}

func TestWaitReadyNamesTheSlotWhoseRemappedHintMismatches(t *testing.T) {
	swapStartupFatalRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "inspect" {
			return []byte("4\n"), nil
		}
		return []byte("PartyId hint featuredapp-validator-1 does not match configured hint myapp-validator-1\n"), nil
	})
	server := notReadyServer(t)
	config := filepath.Join(t.TempDir(), "canton-localnet.yaml")
	body := "schemaVersion: preview-1\nvalidators:\n  a-validator-1:\n    partyHint: myapp-validator-1\n"
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := executeWaitReady(t, "wait-ready", "--config", config, "--url", server.URL, "--slot", "c-validator-1", "--timeout", "30s", "--interval", "1ms")

	want := "party hint featuredapp-validator-1 but the configured hint is myapp-validator-1 (slot a-validator-1)."
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want error containing\n%s\ngot\n%v", want, err)
	}
}

func TestWaitReadyNamesNoSlotWhenTheFatalLineCarriesNoHints(t *testing.T) {
	swapStartupFatalRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "inspect" {
			return []byte("2\n"), nil
		}
		return []byte("does not match configured hint\n"), nil
	})
	server := notReadyServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--slot", "c-validator-1", "--timeout", "30s", "--interval", "1ms")

	want := "the volume was bootstrapped with a different party hint than the configured one. Remove the stale volumes"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want error containing\n%s\ngot\n%v", want, err)
	}
	if strings.Contains(err.Error(), "slot ") {
		t.Fatalf("want no slot named, got %v", err)
	}
}

func TestWaitReadyKeepsTimingOutWhenSpliceHasNeverRestarted(t *testing.T) {
	swapStartupFatalRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("0\n"), nil
	})
	server := notReadyServer(t)

	_, err := executeWaitReady(t, "wait-ready", "--url", server.URL, "--timeout", "50ms", "--interval", "5ms")

	if err == nil || !strings.Contains(err.Error(), "timed out waiting for readiness") {
		t.Fatalf("want the readiness timeout, got %v", err)
	}
}
