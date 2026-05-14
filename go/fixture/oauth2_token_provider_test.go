// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuth2TokenProvider_RequestShape(t *testing.T) {
	var captured struct {
		method      string
		contentType string
		grantType   string
		clientID    string
		secret      string
		audience    string
		scope       string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured.method = r.Method
		captured.contentType = r.Header.Get("Content-Type")
		captured.grantType = r.PostFormValue("grant_type")
		captured.clientID = r.PostFormValue("client_id")
		captured.secret = r.PostFormValue("client_secret")
		captured.audience = r.PostFormValue("audience")
		captured.scope = r.PostFormValue("scope")
		writeToken(t, w, "tok-1", 300)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "client-x", "secret-x", "aud-x", "scope-x", staticClock(time.Unix(1_700_000_000, 0)))

	token, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "tok-1" {
		t.Fatalf("token = %q, want %q", token, "tok-1")
	}
	if captured.method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if captured.contentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", captured.contentType)
	}
	if captured.grantType != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", captured.grantType)
	}
	if captured.clientID != "client-x" {
		t.Errorf("client_id = %q, want client-x", captured.clientID)
	}
	if captured.secret != "secret-x" {
		t.Errorf("client_secret = %q, want secret-x", captured.secret)
	}
	if captured.audience != "aud-x" {
		t.Errorf("audience = %q, want aud-x", captured.audience)
	}
	if captured.scope != "scope-x" {
		t.Errorf("scope = %q, want scope-x", captured.scope)
	}
}

func TestOAuth2TokenProvider_OmitsAudienceAndScopeWhenEmpty(t *testing.T) {
	var form map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		form = r.PostForm
		writeToken(t, w, "tok", 60)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "", "", staticClock(time.Unix(0, 0)))
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if _, ok := form["audience"]; ok {
		t.Errorf("audience should be omitted, got %v", form["audience"])
	}
	if _, ok := form["scope"]; ok {
		t.Errorf("scope should be omitted, got %v", form["scope"])
	}
}

func TestOAuth2TokenProvider_CachesUntilExpiry(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		writeToken(t, w, fmt.Sprintf("tok-%d", n), 300)
	}))
	t.Cleanup(server.Close)

	now := time.Unix(1_700_000_000, 0)
	clock := newControllableClock(now)
	p := newProvider(t, server.URL, "id", "secret", "aud", "", clock.Now)

	for i := 0; i < 5; i++ {
		token, err := p.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if token != "tok-1" {
			t.Fatalf("iteration %d: token = %q, want tok-1", i, token)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("token endpoint hit %d times, want 1", got)
	}
}

func TestOAuth2TokenProvider_RefreshesBeforeNominalExpiryBySkew(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		writeToken(t, w, fmt.Sprintf("tok-%d", n), 60)
	}))
	t.Cleanup(server.Close)

	clock := newControllableClock(time.Unix(1_700_000_000, 0))
	p := newProvider(t, server.URL, "id", "secret", "aud", "", clock.Now)

	first, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	if first != "tok-1" {
		t.Fatalf("first = %q", first)
	}

	clock.Advance(60*time.Second - RefreshSkew - time.Second)
	stillCached, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token after near-expiry advance: %v", err)
	}
	if stillCached != "tok-1" {
		t.Fatalf("expected cached token tok-1, got %q", stillCached)
	}

	clock.Advance(2 * time.Second)
	refreshed, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token after expiry advance: %v", err)
	}
	if refreshed != "tok-2" {
		t.Fatalf("expected refreshed token tok-2, got %q", refreshed)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 token endpoint hits, got %d", got)
	}
}

func TestOAuth2TokenProvider_PropagatesHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"bad creds"}`)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") || !strings.Contains(msg, "invalid_client") || !strings.Contains(msg, "bad creds") {
		t.Errorf("error message %q does not include HTTP status or OAuth error payload", msg)
	}
}

func TestOAuth2TokenProvider_PropagatesTransportErrors(t *testing.T) {
	p := newProvider(t, "http://127.0.0.1:0/no-listener", "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if !strings.Contains(err.Error(), "token request") {
		t.Errorf("error %q does not wrap transport error", err.Error())
	}
}

func TestOAuth2TokenProvider_RejectsNonPositiveExpiresIn(t *testing.T) {
	cases := []struct {
		name      string
		expiresIn any
	}{
		{"zero", 0},
		{"negative", -10},
		{"missing", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				payload := map[string]any{"access_token": "tok", "token_type": "Bearer"}
				if tc.expiresIn != nil {
					payload["expires_in"] = tc.expiresIn
				}
				_ = json.NewEncoder(w).Encode(payload)
			}))
			t.Cleanup(server.Close)

			p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
			_, err := p.Token(context.Background())
			if err == nil {
				t.Fatal("expected error for non-positive expires_in")
			}
			if !strings.Contains(err.Error(), "expires_in") {
				t.Errorf("error %q does not mention expires_in", err.Error())
			}
		})
	}
}

func TestOAuth2TokenProvider_TruncatesLongErrorBody(t *testing.T) {
	long := strings.Repeat("X", 12000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, long)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "...[truncated]") {
		t.Errorf("error %q should be truncated", err.Error())
	}
	if len(err.Error()) > 1024 {
		t.Errorf("error length %d exceeds expected cap (~512 + prefix)", len(err.Error()))
	}
}

func TestOAuth2TokenProvider_RejectsEmptyAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"","expires_in":300}`)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for empty access_token")
	}
}

func TestOAuth2TokenProvider_ConcurrentCallersShareSingleFetch(t *testing.T) {
	var hits int32
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-gate
		writeToken(t, w, "tok-shared", 300)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))

	const callers = 16
	var wg sync.WaitGroup
	tokens := make([]string, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = p.Token(context.Background())
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if tokens[i] != "tok-shared" {
			t.Fatalf("caller %d: token %q", i, tokens[i])
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 token endpoint hit under concurrency, got %d", got)
	}
}

func TestOAuth2TokenProvider_CoalescesRefreshAfterCachedExpiry(t *testing.T) {
	var hits int32
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n > 1 {
			<-gate
		}
		writeToken(t, w, fmt.Sprintf("tok-%d", n), 60)
	}))
	t.Cleanup(server.Close)

	clock := newControllableClock(time.Unix(1_700_000_000, 0))
	p := newProvider(t, server.URL, "id", "secret", "aud", "", clock.Now)

	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("priming Token: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 hit after priming, got %d", got)
	}

	clock.Advance(60 * time.Second)

	const callers = 16
	var wg sync.WaitGroup
	tokens := make([]string, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = p.Token(context.Background())
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if tokens[i] != "tok-2" {
			t.Fatalf("caller %d: token = %q, want tok-2", i, tokens[i])
		}
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 server hits (1 priming + 1 coalesced refresh), got %d", got)
	}
}

func TestOAuth2TokenProvider_FollowerRetriesOnLeaderContextCancel(t *testing.T) {
	var hits int32
	leaderEntered := make(chan struct{})
	releaseLeader := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			close(leaderEntered)
			<-releaseLeader
			writeToken(t, w, "leader-tok", 300)
			return
		}
		writeToken(t, w, "follower-tok", 300)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))

	leaderCtx, cancelLeader := context.WithCancel(context.Background())

	leaderDone := make(chan error, 1)
	go func() {
		_, err := p.Token(leaderCtx)
		leaderDone <- err
	}()

	<-leaderEntered

	type followerResult struct {
		token string
		err   error
	}
	followerDone := make(chan followerResult, 1)
	go func() {
		tok, err := p.Token(context.Background())
		followerDone <- followerResult{tok, err}
	}()

	time.Sleep(50 * time.Millisecond)

	cancelLeader()
	close(releaseLeader)

	select {
	case err := <-leaderDone:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("leader err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not return within 2s")
	}

	select {
	case res := <-followerDone:
		if res.err != nil {
			t.Fatalf("follower err = %v, want nil — leader's context cancellation must not bleed", res.err)
		}
		if res.token != "follower-tok" {
			t.Fatalf("follower token = %q, want follower-tok", res.token)
		}
		if h := atomic.LoadInt32(&hits); h != 2 {
			t.Fatalf("server hits = %d, want 2 (leader cancelled, follower retried)", h)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower did not complete within 2s")
	}
}

func TestOAuth2TokenProvider_InvalidateForcesRefresh(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		writeToken(t, w, fmt.Sprintf("tok-%d", n), 300)
	}))
	t.Cleanup(server.Close)

	p := newProvider(t, server.URL, "id", "secret", "aud", "", staticClock(time.Unix(0, 0)))
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	p.Invalidate()
	second, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token after Invalidate: %v", err)
	}
	if second != "tok-2" {
		t.Fatalf("token after invalidate = %q, want tok-2", second)
	}
}

func TestOAuth2TokenProvider_RequiresCredentials(t *testing.T) {
	cases := []struct {
		name string
		cfg  OAuth2TokenProviderConfig
	}{
		{"missing url", OAuth2TokenProviderConfig{ClientID: "id", ClientSecret: "secret"}},
		{"missing client id", OAuth2TokenProviderConfig{TokenURL: "http://x", ClientSecret: "secret"}},
		{"missing client secret", OAuth2TokenProviderConfig{TokenURL: "http://x", ClientID: "id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewOAuth2TokenProvider(tc.cfg); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func writeToken(t *testing.T, w http.ResponseWriter, accessToken string, expiresInSeconds int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   expiresInSeconds,
	}); err != nil {
		t.Fatalf("encode token: %v", err)
	}
}

func newProvider(t *testing.T, tokenURL, clientID, secret, audience, scope string, now func() time.Time) *OAuth2TokenProvider {
	t.Helper()
	p, err := NewOAuth2TokenProvider(OAuth2TokenProviderConfig{
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: secret,
		Audience:     audience,
		Scope:        scope,
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		Now:          now,
	})
	if err != nil {
		t.Fatalf("NewOAuth2TokenProvider: %v", err)
	}
	return p
}

type controllableClock struct {
	mu  sync.Mutex
	now time.Time
}

func newControllableClock(start time.Time) *controllableClock {
	return &controllableClock{now: start}
}

func (c *controllableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *controllableClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func staticClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

