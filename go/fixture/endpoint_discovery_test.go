// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"strings"
	"testing"
)

func TestEndpointDiscovery_Defaults(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(func(string) string { return "" })

	cases := []struct {
		role      Role
		jsonURL   string
		tokenPath string
		clientID  string
	}{
		{RoleAValidator1, "http://localhost:11975", "/realms/AValidator1/protocol/openid-connect/token", "a-validator-1-validator"},
		{RoleBValidator1, "http://localhost:12975", "/realms/BValidator1/protocol/openid-connect/token", "b-validator-1-validator"},
		{RoleCValidator1, "http://localhost:13975", "/realms/CValidator1/protocol/openid-connect/token", "c-validator-1-validator"},
		{RoleDValidator1, "http://localhost:14975", "/realms/DValidator1/protocol/openid-connect/token", "d-validator-1-validator"},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			e, err := d.For(tc.role)
			if err != nil {
				t.Fatalf("For(%s): %v", tc.role, err)
			}
			if e.JSONLedgerAPIURL != tc.jsonURL {
				t.Errorf("JSONLedgerAPIURL = %q, want %q", e.JSONLedgerAPIURL, tc.jsonURL)
			}
			if !strings.HasSuffix(e.TokenURL, tc.tokenPath) {
				t.Errorf("TokenURL = %q, want suffix %q", e.TokenURL, tc.tokenPath)
			}
			if e.ClientID != tc.clientID {
				t.Errorf("ClientID = %q, want %q", e.ClientID, tc.clientID)
			}
			if e.Audience != "https://canton.network.global" {
				t.Errorf("Audience = %q", e.Audience)
			}
			if e.ClientSecret == "" {
				t.Error("expected non-empty default ClientSecret")
			}
		})
	}
}

func TestEndpointDiscovery_OverridesFromEnv(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_HOST":                       "ledger.example.com",
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":     "9001",
		"CANTON_LOCALNET_KEYCLOAK_HOST":              "auth.example.com",
		"CANTON_LOCALNET_KEYCLOAK_PORT":              "8443",
		"CANTON_LOCALNET_AUDIENCE":                   "https://example.audience",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "custom-client",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "custom-secret",
	}
	d := NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] })
	e, err := d.For(RoleAValidator1)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.JSONLedgerAPIURL != "http://ledger.example.com:9001" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if !strings.HasPrefix(e.TokenURL, "http://auth.example.com:8443/realms/AValidator1/") {
		t.Errorf("TokenURL = %q", e.TokenURL)
	}
	if e.ClientID != "custom-client" {
		t.Errorf("ClientID = %q", e.ClientID)
	}
	if e.ClientSecret != "custom-secret" {
		t.Errorf("ClientSecret = %q", e.ClientSecret)
	}
	if e.Audience != "https://example.audience" {
		t.Errorf("Audience = %q", e.Audience)
	}
}

func TestEndpointDiscovery_UnknownRole(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(func(string) string { return "" })
	if _, err := d.For(Role("nope")); err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestEndpointDiscovery_RoleSvValidator1RequiresClientSecret(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(func(string) string { return "" })
	_, err := d.For(RoleSvValidator1)
	if err == nil {
		t.Fatal("expected error when CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET is unset")
	}
	if !strings.Contains(err.Error(), "CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET") {
		t.Errorf("err = %q, want it to mention the env var", err.Error())
	}
}

func TestEndpointDiscovery_RoleSvValidator1ResolvesWithExplicitSecret(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET": "sv-secret",
	}
	d := NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] })
	e, err := d.For(RoleSvValidator1)
	if err != nil {
		t.Fatalf("For(SV): %v", err)
	}
	if e.ClientSecret != "sv-secret" {
		t.Fatalf("ClientSecret = %q, want sv-secret", e.ClientSecret)
	}
	if e.JSONLedgerAPIURL != "http://localhost:10975" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if !strings.HasSuffix(e.TokenURL, "/realms/sv-validator-1/protocol/openid-connect/token") {
		t.Errorf("TokenURL = %q", e.TokenURL)
	}
}
