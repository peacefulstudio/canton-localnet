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

func TestUserBuilder_SendsRightsInTheSingleCreateRequest(t *testing.T) {
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
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1 (user with rights)", len(calls))
	}
	if calls[0].method != http.MethodPost || calls[0].path != "/v2/users" {
		t.Errorf("call = %s %s, want POST /v2/users", calls[0].method, calls[0].path)
	}
	var payload createUserRequest
	if err := json.Unmarshal(calls[0].body, &payload); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if payload.User.ID != "bob" {
		t.Errorf("user.id = %q", payload.User.ID)
	}
	if len(payload.Rights) != 3 {
		t.Fatalf("rights count = %d, want 3", len(payload.Rights))
	}
	if payload.Rights[0].Kind.CanActAs == nil || payload.Rights[0].Kind.CanActAs.Value.Party != "alice::ns" {
		t.Errorf("rights[0] = %+v, want CanActAs alice::ns", payload.Rights[0])
	}
	if payload.Rights[1].Kind.CanActAs == nil || payload.Rights[1].Kind.CanActAs.Value.Party != "bob::ns" {
		t.Errorf("rights[1] = %+v, want CanActAs bob::ns", payload.Rights[1])
	}
	if payload.Rights[2].Kind.CanReadAs == nil || payload.Rights[2].Kind.CanReadAs.Value.Party != "carol::ns" {
		t.Errorf("rights[2] = %+v, want CanReadAs carol::ns", payload.Rights[2])
	}
}

func TestUserBuilder_PropagatesCreateFailure(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
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
	if requests != 1 {
		t.Errorf("server saw %d requests, want 1", requests)
	}
}

func TestUserBuilder_PropagatesRejectedRights(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `denied`)
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
