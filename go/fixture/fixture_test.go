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

func TestFixture_SetupWiresTokenAndAdminAgainstStub(t *testing.T) {
	var sawAuthHeader string
	ledger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::stub"}`)
	}))
	t.Cleanup(ledger.Close)

	keycloak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.PostFormValue("grant_type") != "client_credentials" {
			http.Error(w, "wrong grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-stub",
			"token_type":   "Bearer",
			"expires_in":   300,
		})
	}))
	t.Cleanup(keycloak.Close)

	ledgerHost, ledgerPort := splitHostPort(t, ledger.URL)
	keycloakHost, keycloakPort := splitHostPort(t, keycloak.URL)

	env := map[string]string{
		"CANTON_LOCALNET_HOST":                       ledgerHost,
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":     ledgerPort,
		"CANTON_LOCALNET_KEYCLOAK_HOST":              keycloakHost,
		"CANTON_LOCALNET_KEYCLOAK_PORT":              keycloakPort,
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "cid",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "csecret",
	}
	f, err := New(Config{
		Role:      RoleAValidator1,
		Discovery: NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if err := f.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = f.Teardown(context.Background()) })

	id, err := f.GetParticipantId(ctx)
	if err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if id != "participant::stub" {
		t.Fatalf("id = %q", id)
	}
	if sawAuthHeader != "Bearer tok-stub" {
		t.Errorf("ledger Authorization header = %q, want Bearer tok-stub", sawAuthHeader)
	}
}

func TestFixture_AccessorsPanicBeforeSetup(t *testing.T) {
	f, err := New(Config{
		Role:      RoleAValidator1,
		Discovery: NewEndpointDiscoveryWithEnv(func(string) string { return "" }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		name        string
		wantMention string
		call        func()
	}{
		{"Endpoints", "Endpoints", func() { _ = f.Endpoints() }},
		{"Tokens", "Tokens", func() { _ = f.Tokens() }},
		{"Admin", "Admin", func() { _ = f.Admin() }},
		{"GetParticipantId", "Admin", func() { _, _ = f.GetParticipantId(context.Background()) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected panic from %s before Setup", tc.name)
				}
				msg, ok := r.(string)
				if !ok {
					t.Fatalf("panic value = %v (%T), want string", r, r)
				}
				if !strings.Contains(msg, "Setup must be called before "+tc.wantMention) {
					t.Errorf("panic msg = %q, want it to mention %s", msg, tc.wantMention)
				}
			}()
			tc.call()
		})
	}
}

func TestFixture_TeardownHonorsCtxCancel(t *testing.T) {
	f, err := New(Config{
		Role:      RoleAValidator1,
		Discovery: NewEndpointDiscoveryWithEnv(func(string) string { return "" }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.Teardown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Teardown(cancelled) = %v, want context.Canceled", err)
	}
}

func splitHostPort(t *testing.T, rawURL string) (host string, port string) {
	t.Helper()
	stripped := strings.TrimPrefix(rawURL, "http://")
	stripped = strings.TrimPrefix(stripped, "https://")
	idx := strings.Index(stripped, ":")
	if idx < 0 {
		t.Fatalf("no port in %q", rawURL)
	}
	return stripped[:idx], stripped[idx+1:]
}
