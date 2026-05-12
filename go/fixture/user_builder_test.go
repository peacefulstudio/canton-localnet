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
	"sync"
	"testing"
)

type recordedCall struct {
	method string
	path   string
	body   []byte
}

func TestUserBuilder_CreateUserRequestShape(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []recordedCall
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, recordedCall{method: r.Method, path: r.URL.Path, body: body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"user":{"id":"alice"}}`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok-user"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	id, err := b.Create(context.Background(), UserOptions{
		UserID:       "alice",
		PrimaryParty: "alice::ns",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "alice" {
		t.Errorf("id = %q, want alice", id)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1 (no rights → no second POST)", len(calls))
	}
	if calls[0].method != http.MethodPost {
		t.Errorf("method = %q", calls[0].method)
	}
	if calls[0].path != "/v2/users" {
		t.Errorf("path = %q", calls[0].path)
	}
	var payload createUserRequest
	if err := json.Unmarshal(calls[0].body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.User.ID != "alice" {
		t.Errorf("user.id = %q", payload.User.ID)
	}
	if payload.User.PrimaryParty != "alice::ns" {
		t.Errorf("user.primaryParty = %q", payload.User.PrimaryParty)
	}
	if payload.User.IsDeactivated {
		t.Errorf("user.isDeactivated = true, want false")
	}
	if payload.User.IdentityProviderID != "" {
		t.Errorf("user.identityProviderId = %q, want empty", payload.User.IdentityProviderID)
	}
	if payload.Rights == nil {
		t.Error("rights should be an empty array, not null")
	}
}

func TestUserBuilder_GrantsActAsAndReadAs(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []recordedCall
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, recordedCall{method: r.Method, path: r.URL.Path, body: body})
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/rights") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"newlyGrantedRights":[]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"user":{"id":"bob"}}`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	_, err = b.Create(context.Background(), UserOptions{
		UserID:       "bob",
		PrimaryParty: "bob::ns",
		ActAs:        []string{"alice::ns", "bob::ns"},
		ReadAs:       []string{"carol::ns"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2 (user + rights)", len(calls))
	}
	if calls[1].path != "/v2/users/bob/rights" {
		t.Errorf("rights path = %q, want /v2/users/bob/rights", calls[1].path)
	}
	var grant grantRightsRequest
	if err := json.Unmarshal(calls[1].body, &grant); err != nil {
		t.Fatalf("decode grant: %v", err)
	}
	if grant.UserID != "bob" {
		t.Errorf("rights.userId = %q", grant.UserID)
	}
	if len(grant.Rights) != 3 {
		t.Fatalf("rights count = %d, want 3", len(grant.Rights))
	}
	if grant.Rights[0].Kind.CanActAs == nil || grant.Rights[0].Kind.CanActAs.Value.Party != "alice::ns" {
		t.Errorf("rights[0] = %+v, want CanActAs alice::ns", grant.Rights[0])
	}
	if grant.Rights[1].Kind.CanActAs == nil || grant.Rights[1].Kind.CanActAs.Value.Party != "bob::ns" {
		t.Errorf("rights[1] = %+v, want CanActAs bob::ns", grant.Rights[1])
	}
	if grant.Rights[2].Kind.CanReadAs == nil || grant.Rights[2].Kind.CanReadAs.Value.Party != "carol::ns" {
		t.Errorf("rights[2] = %+v, want CanReadAs carol::ns", grant.Rights[2])
	}
}

func TestUserBuilder_OrdersCreateBeforeRights(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		order = append(order, r.URL.Path)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/rights") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"user":{"id":"c"}}`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	if _, err := b.Create(context.Background(), UserOptions{UserID: "c", PrimaryParty: "c::ns", ActAs: []string{"c::ns"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "/v2/users" || order[1] != "/v2/users/c/rights" {
		t.Fatalf("call order = %v, want [/v2/users /v2/users/c/rights]", order)
	}
}

func TestUserBuilder_PropagatesCreateFailureBeforeRights(t *testing.T) {
	var rightsHit int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/rights") {
			rightsHit++
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	_, err = b.Create(context.Background(), UserOptions{UserID: "x", PrimaryParty: "x::ns", ActAs: []string{"x::ns"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q missing 500", err.Error())
	}
	if rightsHit != 0 {
		t.Errorf("rights endpoint hit %d times, want 0 — create failure must short-circuit", rightsHit)
	}
}

func TestUserBuilder_PropagatesRightsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/rights") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `denied`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"user":{"id":"x"}}`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	_, err = b.Create(context.Background(), UserOptions{UserID: "x", PrimaryParty: "x::ns", ActAs: []string{"x::ns"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q missing 403", err.Error())
	}
}

func TestUserBuilder_WritesAnnotations(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"user":{"id":"a"}}`)
	}))
	t.Cleanup(server.Close)

	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	_, err = b.Create(context.Background(), UserOptions{
		UserID:       "a",
		PrimaryParty: "a::ns",
		Annotations:  map[string]string{"username": "alice"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var decoded createUserRequest
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.User.Metadata.Annotations["username"] != "alice" {
		t.Errorf("annotations = %+v", decoded.User.Metadata.Annotations)
	}
}

func TestUserBuilder_PropagatesTokenError(t *testing.T) {
	b, err := NewUserBuilder("http://localhost", staticTokenProvider{err: errors.New("nope")})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	_, err = b.Create(context.Background(), UserOptions{UserID: "x", PrimaryParty: "x::ns"})
	if err == nil {
		t.Fatal("expected token error")
	}
}

func TestUserBuilder_HonorsContextCancel(t *testing.T) {
	b, err := NewUserBuilder("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = b.Create(ctx, UserOptions{UserID: "x", PrimaryParty: "x::ns"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create(cancelled) = %v, want context.Canceled", err)
	}
}

func TestUserBuilder_RequiresArguments(t *testing.T) {
	if _, err := NewUserBuilder("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewUserBuilder("http://x", nil); err == nil {
		t.Error("expected error for nil tokens")
	}
	b, _ := NewUserBuilder("http://x", staticTokenProvider{token: "tok"})
	if _, err := b.Create(context.Background(), UserOptions{PrimaryParty: "x::ns"}); err == nil {
		t.Error("expected error for empty UserID")
	}
	if _, err := b.Create(context.Background(), UserOptions{UserID: "x"}); err == nil {
		t.Error("expected error for empty PrimaryParty")
	}
}
