// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticTokenProvider struct {
	token string
	err   error
}

func (s staticTokenProvider) Token(ctx context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.token, nil
}

func TestJsonLedgerAdminClient_GetParticipantId_RequestShape(t *testing.T) {
	var (
		method string
		path   string
		auth   string
		accept string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::abc123"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok-xyz"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}

	id, err := client.GetParticipantId(context.Background())
	if err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if id != "participant::abc123" {
		t.Fatalf("id = %q, want participant::abc123", id)
	}
	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
	if path != "/v2/parties/participant-id" {
		t.Errorf("path = %q, want /v2/parties/participant-id", path)
	}
	if auth != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer tok-xyz")
	}
	if accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", accept)
	}
}

func TestJsonLedgerAdminClient_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::abc"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL+"/", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if path != "/v2/parties/participant-id" {
		t.Errorf("path = %q (got double slash?)", path)
	}
}

func TestJsonLedgerAdminClient_PropagatesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"forbidden"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	_, err = client.GetParticipantId(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not include 403", err.Error())
	}
}

func TestJsonLedgerAdminClient_RejectsEmptyParticipantId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":""}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err == nil {
		t.Fatal("expected error for empty participantId")
	}
}

func TestJsonLedgerAdminClient_PropagatesTokenError(t *testing.T) {
	client, err := NewJsonLedgerAdminClient("http://localhost", staticTokenProvider{err: errors.New("boom")})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	_, err = client.GetParticipantId(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "acquire token") {
		t.Errorf("error %q does not wrap token-acquisition failure", err.Error())
	}
}

func TestJsonLedgerAdminClient_RejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `not json`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestJsonLedgerAdminClient_RequiresArguments(t *testing.T) {
	if _, err := NewJsonLedgerAdminClient("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewJsonLedgerAdminClient("http://x", nil); err == nil {
		t.Error("expected error for nil TokenProvider")
	}
}
