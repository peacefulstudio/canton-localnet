// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPartyAllocator_RequestShape(t *testing.T) {
	var captured struct {
		method      string
		path        string
		auth        string
		contentType string
		body        partyAllocateRequest
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.auth = r.Header.Get("Authorization")
		captured.contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"partyDetails":{"party":"alice::ns1"}}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok-party"}, WithPartyAllocatorSuffix("abc123"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	got, err := p.Allocate(context.Background(), "globex", "Alice")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "alice::ns1" {
		t.Errorf("party = %q, want alice::ns1", got)
	}
	if captured.method != http.MethodPost {
		t.Errorf("method = %q", captured.method)
	}
	if captured.path != "/v2/parties" {
		t.Errorf("path = %q", captured.path)
	}
	if captured.auth != "Bearer tok-party" {
		t.Errorf("auth = %q", captured.auth)
	}
	if captured.contentType != "application/json" {
		t.Errorf("Content-Type = %q", captured.contentType)
	}
	if captured.body.PartyIDHint != "globex-abc123" {
		t.Errorf("partyIdHint = %q, want globex-abc123", captured.body.PartyIDHint)
	}
	if captured.body.DisplayName != "Alice" {
		t.Errorf("displayName = %q, want Alice", captured.body.DisplayName)
	}
}

func TestPartyAllocator_HintFollowsPrefixSuffixFormat(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("ZZZ"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if got := p.Hint("initech"); got != "initech-ZZZ" {
		t.Errorf("Hint(initech) = %q, want initech-ZZZ", got)
	}
	if got := p.Hint("globex"); got != "globex-ZZZ" {
		t.Errorf("Hint(globex) = %q, want globex-ZZZ", got)
	}
	if p.Suffix() != "ZZZ" {
		t.Errorf("Suffix = %q, want ZZZ", p.Suffix())
	}
}

func TestPartyAllocator_GeneratesUniqueSuffixPerInstance(t *testing.T) {
	a, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if a.Suffix() == "" || b.Suffix() == "" {
		t.Fatal("expected non-empty suffixes")
	}
	if a.Suffix() == b.Suffix() {
		t.Fatalf("suffixes should differ between instances: a=%q b=%q", a.Suffix(), b.Suffix())
	}
	if len(a.Suffix()) < 8 {
		t.Errorf("suffix %q is suspiciously short (need at least 8 hex chars)", a.Suffix())
	}
}

func TestPartyAllocator_SuffixIsStableAcrossCalls(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	first := p.Hint("globex")
	for i := 0; i < 5; i++ {
		if p.Hint("globex") != first {
			t.Fatalf("hint changed across calls: %q vs %q", first, p.Hint("globex"))
		}
	}
}

func TestPartyAllocator_OmitsDisplayNameWhenEmpty(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rawBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"partyDetails":{"party":"x::y"}}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("sfx"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if _, err := p.Allocate(context.Background(), "initech", ""); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if strings.Contains(rawBody, "displayName") {
		t.Errorf("displayName should be omitted when empty; body = %s", rawBody)
	}
}

func TestPartyAllocator_PropagatesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"cause":"PARTY_ALREADY_EXISTS"}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	_, err = p.Allocate(context.Background(), "initech", "")
	if err == nil {
		t.Fatal("expected error on 409")
	}
	if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "PARTY_ALREADY_EXISTS") {
		t.Errorf("error %q missing status or cause", err.Error())
	}
}

func TestPartyAllocator_RejectsEmptyPartyInResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"partyDetails":{"party":""}}`)
	}))
	t.Cleanup(server.Close)

	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	_, err = p.Allocate(context.Background(), "initech", "")
	if err == nil {
		t.Fatal("expected error for empty party in response")
	}
}

func TestPartyAllocator_RejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `not json`)
	}))
	t.Cleanup(server.Close)

	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if _, err := p.Allocate(context.Background(), "initech", ""); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestPartyAllocator_PropagatesTokenError(t *testing.T) {
	p, err := NewPartyAllocator("http://localhost", staticTokenProvider{err: errors.New("no token")}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if _, err := p.Allocate(context.Background(), "initech", ""); err == nil {
		t.Fatal("expected token error")
	}
}

func TestPartyAllocator_HonorsContextCancel(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Allocate(ctx, "initech", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("Allocate(cancelled) = %v, want context.Canceled", err)
	}
}

func TestPartyAllocator_RequiresArguments(t *testing.T) {
	if _, err := NewPartyAllocator("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewPartyAllocator("http://x", nil); err == nil {
		t.Error("expected error for nil tokens")
	}
	p, _ := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if _, err := p.Allocate(context.Background(), "", ""); err == nil {
		t.Error("expected error for empty prefix")
	}
}
