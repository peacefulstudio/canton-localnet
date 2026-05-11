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
		{RoleAppProvider, "http://localhost:3975", "/realms/AppProvider/protocol/openid-connect/token", "app-provider-validator"},
		{RoleAppUser, "http://localhost:2975", "/realms/AppUser/protocol/openid-connect/token", "app-user-validator"},
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
		"CANTON_LOCALNET_APP_PROVIDER_JSON_PORT":     "9001",
		"CANTON_LOCALNET_KEYCLOAK_HOST":              "auth.example.com",
		"CANTON_LOCALNET_KEYCLOAK_PORT":              "8443",
		"CANTON_LOCALNET_AUDIENCE":                   "https://example.audience",
		"CANTON_LOCALNET_APP_PROVIDER_CLIENT_ID":     "custom-client",
		"CANTON_LOCALNET_APP_PROVIDER_CLIENT_SECRET": "custom-secret",
	}
	d := NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] })
	e, err := d.For(RoleAppProvider)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.JSONLedgerAPIURL != "http://ledger.example.com:9001" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if !strings.HasPrefix(e.TokenURL, "http://auth.example.com:8443/realms/AppProvider/") {
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

func TestEndpointDiscovery_RoleSVRequiresClientSecret(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(func(string) string { return "" })
	_, err := d.For(RoleSV)
	if err == nil {
		t.Fatal("expected error when CANTON_LOCALNET_SV_CLIENT_SECRET is unset")
	}
	if !strings.Contains(err.Error(), "CANTON_LOCALNET_SV_CLIENT_SECRET") {
		t.Errorf("err = %q, want it to mention the env var", err.Error())
	}
}

func TestEndpointDiscovery_RoleSVResolvesWithExplicitSecret(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_SV_CLIENT_SECRET": "sv-secret",
	}
	d := NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] })
	e, err := d.For(RoleSV)
	if err != nil {
		t.Fatalf("For(SV): %v", err)
	}
	if e.ClientSecret != "sv-secret" {
		t.Fatalf("ClientSecret = %q, want sv-secret", e.ClientSecret)
	}
	if e.JSONLedgerAPIURL != "http://localhost:4975" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if !strings.HasSuffix(e.TokenURL, "/realms/sv/protocol/openid-connect/token") {
		t.Errorf("TokenURL = %q", e.TokenURL)
	}
}
