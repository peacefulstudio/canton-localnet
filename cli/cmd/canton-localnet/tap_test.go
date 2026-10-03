// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type recordedRequest struct {
	method string
	host   string
	path   string
	auth   string
	body   string
}

type fakeKeycloakAndWallet struct {
	mu       sync.Mutex
	requests []recordedRequest
	tapCode  int
}

func (f *fakeKeycloakAndWallet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{method: r.Method, host: r.Host, path: r.URL.Path, auth: r.Header.Get("Authorization"), body: string(raw)})
	f.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/openid-connect/token") {
		_, _ = w.Write([]byte(`{"access_token":"wallet-jwt"}`))
		return
	}
	if f.tapCode != 0 {
		w.WriteHeader(f.tapCode)
		_, _ = w.Write([]byte(`wallet said no`))
		return
	}
	_, _ = w.Write([]byte(`{"contract_id":"00new-amulet"}`))
}

func routedTo(server *httptest.Server) *http.Client {
	target, _ := url.Parse(server.URL)
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rewritten := r.Clone(r.Context())
		rewritten.URL.Scheme = target.Scheme
		rewritten.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(rewritten)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func runTap(t *testing.T, deps tapDeps, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "canton-localnet", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("repo-root", t.TempDir(), "")
	root.PersistentFlags().String("config", "", "")
	root.AddCommand(newTapCommand(deps))
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"tap"}, args...))
	err := root.Execute()
	return stdout.String(), err
}

func TestTap_logs_in_as_the_wallet_user_then_taps_the_validator_app(t *testing.T) {
	fake := &fakeKeycloakAndWallet{}
	server := httptest.NewServer(fake)
	defer server.Close()

	stdout, err := runTap(t, tapDeps{httpClient: routedTo(server)}, "--slot", "a", "--amount", "10")
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(stdout); got != "00new-amulet" {
		t.Fatalf("stdout = %q, want the contract id only", got)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("requests = %d, want a login then a tap", len(fake.requests))
	}
	login, tap := fake.requests[0], fake.requests[1]
	if login.host != "localhost:8082" || login.path != "/realms/AValidator1/protocol/openid-connect/token" {
		t.Fatalf("login went to %s%s", login.host, login.path)
	}
	const wantForm = "client_id=a-validator-1-unsafe&grant_type=password&password=abc123&scope=openid&username=a-validator-1"
	if login.body != wantForm {
		t.Fatalf("login form = %s, want %s", login.body, wantForm)
	}
	if tap.method != "POST" || tap.host != "localhost:11903" || tap.path != "/api/validator/v0/wallet/tap" {
		t.Fatalf("tap went to %s %s%s", tap.method, tap.host, tap.path)
	}
	if tap.auth != "Bearer wallet-jwt" || tap.body != `{"amount":"10"}` {
		t.Fatalf("tap auth/body = %q / %s", tap.auth, tap.body)
	}
}

func TestTap_targets_each_slots_own_realm_and_validator_port(t *testing.T) {
	cases := []struct {
		slot, realm, tapHost, user, client string
	}{
		{"b", "BValidator1", "localhost:12903", "b-validator-1", "b-validator-1-unsafe"},
		{"c", "CValidator1", "localhost:13903", "c-validator-1", "c-validator-1-unsafe"},
		{"d", "DValidator1", "localhost:14903", "d-validator-1", "d-validator-1-unsafe"},
	}
	for _, c := range cases {
		fake := &fakeKeycloakAndWallet{}
		server := httptest.NewServer(fake)
		if _, err := runTap(t, tapDeps{httpClient: routedTo(server)}, "--slot", c.slot, "--amount", "1"); err != nil {
			server.Close()
			t.Fatalf("slot %s: %v", c.slot, err)
		}
		server.Close()
		if fake.requests[0].path != "/realms/"+c.realm+"/protocol/openid-connect/token" {
			t.Errorf("slot %s: login path %s", c.slot, fake.requests[0].path)
		}
		if !strings.Contains(fake.requests[0].body, "client_id="+c.client) || !strings.Contains(fake.requests[0].body, "username="+c.user) {
			t.Errorf("slot %s: login form %s", c.slot, fake.requests[0].body)
		}
		if fake.requests[1].host != c.tapHost {
			t.Errorf("slot %s: tap host %s, want %s", c.slot, fake.requests[1].host, c.tapHost)
		}
	}
}

func TestTap_fails_loudly_when_the_wallet_answers_non_2xx(t *testing.T) {
	fake := &fakeKeycloakAndWallet{tapCode: http.StatusBadRequest}
	server := httptest.NewServer(fake)
	defer server.Close()

	stdout, err := runTap(t, tapDeps{httpClient: routedTo(server)}, "--slot", "a", "--amount", "10")

	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "wallet said no") {
		t.Fatalf("error = %v, want one naming HTTP 400 and the body", err)
	}
	if strings.Contains(err.Error(), "wallet-jwt") {
		t.Fatalf("error leaks the wallet token: %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want nothing on failure", stdout)
	}
}

func TestTap_refuses_the_sv_slot_before_any_request(t *testing.T) {
	fake := &fakeKeycloakAndWallet{}
	server := httptest.NewServer(fake)
	defer server.Close()

	for _, name := range []string{"sv", "sv-validator-1"} {
		_, err := runTap(t, tapDeps{httpClient: routedTo(server)}, "--slot", name, "--amount", "10")
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("--slot %s: error = %v, want a refusal", name, err)
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("requests = %d, want none", len(fake.requests))
	}
}

func TestTap_rejects_a_bad_amount_before_any_request(t *testing.T) {
	fake := &fakeKeycloakAndWallet{}
	server := httptest.NewServer(fake)
	defer server.Close()

	for _, amount := range []string{"0", "-3", "abc", "1e3"} {
		if _, err := runTap(t, tapDeps{httpClient: routedTo(server)}, "--slot", "a", "--amount", amount); err == nil {
			t.Fatalf("--amount %s: want an error", amount)
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("requests = %d, want none", len(fake.requests))
	}
}

func TestTap_requires_slot_and_amount(t *testing.T) {
	deps := defaultTapDeps()
	if _, err := runTap(t, deps, "--amount", "1"); err == nil {
		t.Fatal("want an error without --slot")
	}
	if _, err := runTap(t, deps, "--slot", "a"); err == nil {
		t.Fatal("want an error without --amount")
	}
}

func TestTap_amount_and_at_least_are_mutually_exclusive_and_one_is_required(t *testing.T) {
	fake := &fakeKeycloakAndWallet{}
	server := httptest.NewServer(fake)
	defer server.Close()
	deps := tapDeps{httpClient: routedTo(server)}

	if _, err := runTap(t, deps, "--slot", "a", "--amount", "1", "--at-least", "100"); err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("both flags: error = %v, want a mutual exclusivity error", err)
	}
	if _, err := runTap(t, deps, "--slot", "a"); err == nil || !strings.Contains(err.Error(), "at least one of the flags") {
		t.Fatalf("neither flag: error = %v, want a one-required error", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("requests = %d, want none", len(fake.requests))
	}
}

type fakeFundedWallet struct {
	mu      sync.Mutex
	balance string
	taps    []string
}

func (f *fakeFundedWallet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, "/openid-connect/token"):
		_, _ = w.Write([]byte(`{"access_token":"wallet-jwt"}`))
	case strings.HasSuffix(r.URL.Path, "/wallet/balance"):
		_, _ = w.Write([]byte(`{"effective_unlocked_qty":"` + f.balance + `"}`))
	case strings.HasSuffix(r.URL.Path, "/open-and-issuing-mining-rounds"):
		_, _ = w.Write([]byte(`{"open_mining_rounds":[{"contract":{"payload":{"amuletPrice":"0.005","opensAt":"2026-10-03T11:29:26Z"}}}]}`))
	case strings.HasSuffix(r.URL.Path, "/wallet/tap"):
		f.taps = append(f.taps, string(raw))
		f.balance = "1000.0000000000"
		_, _ = w.Write([]byte(`{"contract_id":"00new-amulet"}`))
	}
}

func TestTap_at_least_taps_the_gap_once_then_a_second_run_is_a_no_op(t *testing.T) {
	fake := &fakeFundedWallet{balance: "0.0000000000"}
	server := httptest.NewServer(fake)
	defer server.Close()
	deps := tapDeps{httpClient: routedTo(server), now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }}

	first, err := runTap(t, deps, "--slot", "a", "--at-least", "1000")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runTap(t, deps, "--slot", "a", "--at-least", "1000")
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(first) != "1000.0000000000" || strings.TrimSpace(second) != "1000.0000000000" {
		t.Fatalf("stdout = %q then %q, want the resulting balance each time", first, second)
	}
	if len(fake.taps) != 1 || fake.taps[0] != `{"amount":"5"}` {
		t.Fatalf("taps = %v, want exactly one tap with usd amount \"5\"", fake.taps)
	}
}
