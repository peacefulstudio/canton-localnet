// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package wallet_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/wallet"
)

type scriptedServer struct {
	*httptest.Server
	requests int
}

func serving(t *testing.T, answers ...func(http.ResponseWriter)) *scriptedServer {
	t.Helper()
	scripted := &scriptedServer{}
	scripted.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		index := min(scripted.requests, len(answers)-1)
		scripted.requests++
		answers[index](w)
	}))
	t.Cleanup(scripted.Close)
	return scripted
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func tapped(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"contract_id":"00ok"}`)) }

func recordingSleep(delays *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, delay time.Duration) error {
		*delays = append(*delays, delay)
		return nil
	}
}

func TestTap_retries_a_429_and_succeeds_on_the_next_attempt(t *testing.T) {
	server := serving(t, status(429), tapped)
	var delays []time.Duration

	contractID, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")

	if err != nil || contractID != "00ok" {
		t.Fatalf("contract id, err = %q, %v; want 00ok, nil", contractID, err)
	}
	if server.requests != 2 {
		t.Fatalf("requests = %d, want 2", server.requests)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("delays = %v, want [1s]", delays)
	}
}

func TestTap_does_not_retry_502_503_or_504_because_the_mint_may_have_committed(t *testing.T) {
	for _, code := range []int{502, 503, 504} {
		server := serving(t, status(code), tapped)
		var delays []time.Duration
		_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")
		if err == nil || !strings.Contains(err.Error(), "HTTP "+strconv.Itoa(code)) {
			t.Fatalf("status %d: error = %v, want one naming the status", code, err)
		}
		if server.requests != 1 || len(delays) != 0 {
			t.Fatalf("status %d: requests = %d, sleeps = %d, want a single request", code, server.requests, len(delays))
		}
	}
}

func TestTap_honours_retry_after_seconds(t *testing.T) {
	server := serving(t, func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
	}, tapped)
	var delays []time.Duration

	if _, err := (wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}).Tap(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 1 || delays[0] != 7*time.Second {
		t.Fatalf("delays = %v, want [7s]", delays)
	}
}

func TestTap_backs_off_exponentially_up_to_eight_seconds(t *testing.T) {
	server := serving(t, status(429), status(429), status(429), status(429), status(429), tapped)
	var delays []time.Duration

	if _, err := (wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}).Tap(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
}

func TestTap_a_persistent_429_fails_with_the_last_status_once_the_budget_is_spent(t *testing.T) {
	server := serving(t, status(429))
	var delays []time.Duration

	_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Budget: 10 * time.Second, Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")

	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("error = %v, want one naming HTTP 429", err)
	}
	var total time.Duration
	for _, delay := range delays {
		total += delay
	}
	if total > 10*time.Second {
		t.Fatalf("waited %v, over the 10s budget", total)
	}
	if server.requests != len(delays)+1 {
		t.Fatalf("requests = %d with %d sleeps, want one more request than sleeps", server.requests, len(delays))
	}
}

func TestTap_default_budget_is_sixty_seconds(t *testing.T) {
	server := serving(t, status(429))
	var delays []time.Duration

	_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")

	if err == nil {
		t.Fatal("want an error")
	}
	var total time.Duration
	for _, delay := range delays {
		total += delay
	}
	if total != 55*time.Second {
		t.Fatalf("waited %v in total, want 55s (1+2+4 then 8s steps; the next 8s step would pass 60s)", total)
	}
}

func TestTap_a_retry_after_beyond_the_budget_fails_without_sleeping(t *testing.T) {
	server := serving(t, func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	var delays []time.Duration

	_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")

	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("error = %v, want HTTP 429", err)
	}
	if len(delays) != 0 || server.requests != 1 {
		t.Fatalf("delays = %v, requests = %d; want no sleep and one request", delays, server.requests)
	}
}

func TestTap_does_not_retry_a_400_or_other_4xx(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 500} {
		server := serving(t, status(code))
		var delays []time.Duration

		_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: recordingSleep(&delays)}}.Tap(context.Background(), "1")

		if err == nil {
			t.Fatalf("status %d: want an error", code)
		}
		if server.requests != 1 || len(delays) != 0 {
			t.Fatalf("status %d: requests = %d, sleeps = %d; want exactly one request and no sleep", code, server.requests, len(delays))
		}
	}
}

func TestTap_retries_a_refused_connection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	var delays []time.Duration
	sleep := func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	_, err = wallet.Client{BaseURL: "http://" + address, Token: "t", Retry: wallet.Retry{Budget: 3 * time.Second, Sleep: sleep}}.Tap(context.Background(), "1")

	if err == nil {
		t.Fatal("want an error once the budget is spent")
	}
	if len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
		t.Fatalf("delays = %v, want [1s 2s]", delays)
	}
}

func TestTap_stops_retrying_when_the_context_ends(t *testing.T) {
	server := serving(t, status(429))
	ctx, cancel := context.WithCancel(context.Background())
	sleep := func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := wallet.Client{BaseURL: server.URL, Token: "t", Retry: wallet.Retry{Sleep: sleep}}.Tap(ctx, "1")

	if err == nil || server.requests != 1 {
		t.Fatalf("error = %v, requests = %d; want an error after one request", err, server.requests)
	}
}

func TestMintToken_retries_a_503_and_succeeds(t *testing.T) {
	server := serving(t, status(503), func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"access_token":"jwt-2"}`)) })
	var delays []time.Duration

	token, err := wallet.MintToken(context.Background(), wallet.Credentials{TokenURL: server.URL, ClientID: "c", Username: "u", Password: "p"}, nil, wallet.Retry{Sleep: recordingSleep(&delays)})

	if err != nil || token != "jwt-2" {
		t.Fatalf("token, err = %q, %v; want jwt-2, nil", token, err)
	}
	if server.requests != 2 {
		t.Fatalf("requests = %d, want 2", server.requests)
	}
}

func TestMintToken_does_not_retry_a_401(t *testing.T) {
	server := serving(t, status(401))
	var delays []time.Duration

	_, err := wallet.MintToken(context.Background(), wallet.Credentials{TokenURL: server.URL, ClientID: "c", Username: "u", Password: "p"}, nil, wallet.Retry{Sleep: recordingSleep(&delays)})

	if err == nil || server.requests != 1 {
		t.Fatalf("error = %v, requests = %d; want an error after exactly one request", err, server.requests)
	}
}
