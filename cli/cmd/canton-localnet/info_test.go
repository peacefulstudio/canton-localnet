// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestInfoOfflineEmitsStaticJSON(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")

	stdout := captureRoot(t, "info", "--slot", "a", "--json", "--offline")

	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\nstdout: %s", err, stdout)
	}
	if got.Slot != "a-validator-1" {
		t.Errorf("slot: got %q", got.Slot)
	}
	if got.JSONAPI != "http://localhost:11975" {
		t.Errorf("json_api: got %q", got.JSONAPI)
	}
	if got.LedgerGrpc != "localhost:11901" {
		t.Errorf("ledger_grpc: got %q", got.LedgerGrpc)
	}
	if got.AdminGrpc != "localhost:11902" {
		t.Errorf("admin_grpc: got %q", got.AdminGrpc)
	}
	if got.Realm != "AValidator1" {
		t.Errorf("realm: got %q", got.Realm)
	}
	if got.TokenURLHost != "http://localhost:8082/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("token_url_host: got %q", got.TokenURLHost)
	}
	if got.TokenURLInternal != "http://nginx-keycloak:8082/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("token_url_internal: got %q", got.TokenURLInternal)
	}
	if got.Audience != "https://canton.network.global" {
		t.Errorf("audience: got %q", got.Audience)
	}
	if got.AuthKind != "oauth2" {
		t.Errorf("auth_kind: got %q", got.AuthKind)
	}
	if got.ParticipantID != "" || got.ParticipantNamespace != "" || got.ValidatorPrimaryParty != "" {
		t.Errorf("offline output should not include live participant info, got %+v", got)
	}
}

func TestInfoOnlineHitsParticipantID(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "secret")

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()

	var receivedAuth string
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd1234"}`))
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	jsonHost, jsonPort := splitHostPort(t, jsonServer.URL)
	if tokenHost != jsonHost {
		t.Fatalf("httptest invariant broken: token=%s json=%s — both must share a host", tokenHost, jsonHost)
	}

	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)

	stdout := captureRoot(t, "info", "--slot", "a", "--json")

	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\nstdout: %s", err, stdout)
	}
	if got.ParticipantID != "a-validator-1::abcd1234" {
		t.Errorf("participant_id: got %q", got.ParticipantID)
	}
	if got.ParticipantNamespace != "abcd1234" {
		t.Errorf("participant_namespace: got %q", got.ParticipantNamespace)
	}
	if got.ValidatorPrimaryParty != "a-validator-1::abcd1234" {
		t.Errorf("validator_primary_party: got %q", got.ValidatorPrimaryParty)
	}
	if receivedAuth != "Bearer tok" {
		t.Errorf("authorization header: got %q", receivedAuth)
	}
}

func TestInfoOnlineValidatorPrimaryPartyComesFromParticipantIDNotPartyHint(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "secret")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_PARTY_HINT", "featuredapp-validator-1")

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":300}`))
	}))
	defer tokenServer.Close()
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd1234"}`))
	}))
	defer jsonServer.Close()

	tokenHost, tokenPort := splitHostPort(t, tokenServer.URL)
	_, jsonPort := splitHostPort(t, jsonServer.URL)
	t.Setenv("CANTON_LOCALNET_HOST", tokenHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", tokenPort)
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", jsonPort)

	stdout := captureRoot(t, "info", "--slot", "a", "--json")
	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got.PartyHint != "featuredapp-validator-1" {
		t.Errorf("party_hint should reflect configured override, got %q", got.PartyHint)
	}
	if got.ValidatorPrimaryParty != "a-validator-1::abcd1234" {
		t.Errorf("validator_primary_party must come from participant id's hint, not party_hint; got %q", got.ValidatorPrimaryParty)
	}
}

func TestInfoOfflineJSONSchemaShape(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_HOST", "localhost")
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")

	stdout := captureRoot(t, "info", "--slot", "a", "--json", "--offline")

	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	wantKeys := []string{
		"slot", "json_api", "ledger_grpc", "admin_grpc", "validator_admin",
		"realm", "token_url_host", "token_url_internal",
		"audience", "auth_kind", "party_hint",
	}
	for _, k := range wantKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("expected key %q in JSON output, missing", k)
		}
	}
	for _, k := range []string{"participant_id", "participant_namespace", "validator_primary_party"} {
		if _, ok := raw[k]; ok {
			t.Errorf("offline output should omit %q, got it", k)
		}
	}
}

func TestInfoOfflineHumanOutput(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	stdout := captureRoot(t, "info", "--slot", "a", "--offline")
	for _, expect := range []string{
		"slot", "a-validator-1",
		"realm", "AValidator1",
		"json_api", "http://localhost:11975",
	} {
		if !strings.Contains(stdout, expect) {
			t.Errorf("expected %q in human output, got:\n%s", expect, stdout)
		}
	}
}

func TestInfoOfflinePicksUpYamlPartyHint(t *testing.T) {
	root := t.TempDir()
	cfg := "schemaVersion: preview-1\nvalidators:\n  a-validator-1:\n    partyHint: featuredapp-validator-1\n"
	cfgPath := root + "/canton-localnet.yaml"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")

	stdout := captureRoot(t, "info", "--slot", "a", "--json", "--offline", "--config", cfgPath)
	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PartyHint != "featuredapp-validator-1" {
		t.Errorf("party_hint: got %q, want featuredapp-validator-1", got.PartyHint)
	}
}

func TestInfoOfflineForSVOmitsTokenURLs(t *testing.T) {
	stdout := captureRoot(t, "info", "--slot", "sv", "--json", "--offline")
	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\nstdout: %s", err, stdout)
	}
	if got.TokenURLHost != "" {
		t.Errorf("sv should not have token_url_host, got %q", got.TokenURLHost)
	}
	if got.AuthKind != "hs256" {
		t.Errorf("auth_kind: got %q", got.AuthKind)
	}
	if got.Realm != "" {
		t.Errorf("sv realm should be empty, got %q", got.Realm)
	}
}
