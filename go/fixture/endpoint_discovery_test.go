// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const sharedCasesPath = "../../testdata/endpoint-discovery/cases.json"

type sharedCase struct {
	Name   string            `json:"name"`
	Slot   string            `json:"slot"`
	Env    map[string]string `json:"env"`
	Expect *sharedExpect     `json:"expect"`
	Error  []string          `json:"error"`
}

type sharedExpect struct {
	JSONAPIURL   string `json:"jsonApiUrl"`
	GRPCAPIURL   string `json:"grpcLedgerApiUrl"`
	TokenURL     string `json:"tokenUrl"`
	Audience     string `json:"audience"`
	Scope        string `json:"scope"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

func loadSharedCases(t *testing.T) []sharedCase {
	t.Helper()
	raw, err := os.ReadFile(sharedCasesPath)
	if err != nil {
		t.Fatalf("read shared cases: %v", err)
	}
	var file struct {
		Cases []sharedCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse shared cases: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("shared case table is empty")
	}
	return file.Cases
}

func envLookup(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestEndpointDiscovery_SharedCases(t *testing.T) {
	for _, tc := range loadSharedCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			d := NewEndpointDiscoveryWithEnv(envLookup(tc.Env))
			e, err := d.For(Role(tc.Slot))
			if tc.Expect == nil {
				if err == nil {
					t.Fatalf("For(%s) = %v, want an error", tc.Slot, e)
				}
				for _, want := range tc.Error {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("err = %q, want it to mention %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("For(%s): %v", tc.Slot, err)
			}
			if e.JSONLedgerAPIURL != tc.Expect.JSONAPIURL {
				t.Errorf("JSONLedgerAPIURL = %q, want %q", e.JSONLedgerAPIURL, tc.Expect.JSONAPIURL)
			}
			if e.GRPCLedgerAPIURL != tc.Expect.GRPCAPIURL {
				t.Errorf("GRPCLedgerAPIURL = %q, want %q", e.GRPCLedgerAPIURL, tc.Expect.GRPCAPIURL)
			}
			if e.TokenURL != tc.Expect.TokenURL {
				t.Errorf("TokenURL = %q, want %q", e.TokenURL, tc.Expect.TokenURL)
			}
			if e.Audience != tc.Expect.Audience {
				t.Errorf("Audience = %q, want %q", e.Audience, tc.Expect.Audience)
			}
			if e.Scope != tc.Expect.Scope {
				t.Errorf("Scope = %q, want %q", e.Scope, tc.Expect.Scope)
			}
			if e.ClientID != tc.Expect.ClientID {
				t.Errorf("ClientID = %q, want %q", e.ClientID, tc.Expect.ClientID)
			}
			if tc.Expect.ClientSecret != "" && e.ClientSecret != tc.Expect.ClientSecret {
				t.Errorf("ClientSecret = %q, want %q", e.ClientSecret, tc.Expect.ClientSecret)
			}
			if tc.Expect.ClientSecret == "" && e.ClientSecret == "" {
				t.Error("expected a non-empty default ClientSecret")
			}
		})
	}
}

func TestEndpointDiscovery_HostAndPortCompose(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_HOST":                    "ledger.example.com",
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT": "9001",
		"CANTON_LOCALNET_KEYCLOAK_HOST":           "auth.example.com",
		"CANTON_LOCALNET_KEYCLOAK_PORT":           "8443",
	}
	e, err := NewEndpointDiscoveryWithEnv(envLookup(env)).For(RoleAValidator1)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.JSONLedgerAPIURL != "http://ledger.example.com:9001" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if e.TokenURL != "http://auth.example.com:8443/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("TokenURL = %q", e.TokenURL)
	}
}

func TestEndpointDiscovery_HostAndPortComposeTheGRPCLedgerAPIURL(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_HOST":                    "ledger.example.com",
		"CANTON_LOCALNET_B_VALIDATOR_1_GRPC_PORT": "9101",
	}
	e, err := NewEndpointDiscoveryWithEnv(envLookup(env)).For(RoleBValidator1)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.GRPCLedgerAPIURL != "http://ledger.example.com:9101" {
		t.Errorf("GRPCLedgerAPIURL = %q", e.GRPCLedgerAPIURL)
	}
}

func TestEndpointDiscovery_HostWithoutPortOverrideKeepsSlotDefaultGRPCPort(t *testing.T) {
	env := map[string]string{"CANTON_LOCALNET_HOST": "ledger.example.com"}
	e, err := NewEndpointDiscoveryWithEnv(envLookup(env)).For(RoleCValidator1)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.GRPCLedgerAPIURL != "http://ledger.example.com:13901" {
		t.Errorf("GRPCLedgerAPIURL = %q", e.GRPCLedgerAPIURL)
	}
}

func TestEndpointDiscovery_HostWithoutPortOverrideKeepsSlotDefaultPort(t *testing.T) {
	env := map[string]string{"CANTON_LOCALNET_HOST": "ledger.example.com"}
	e, err := NewEndpointDiscoveryWithEnv(envLookup(env)).For(RoleCValidator1)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if e.JSONLedgerAPIURL != "http://ledger.example.com:13975" {
		t.Errorf("JSONLedgerAPIURL = %q", e.JSONLedgerAPIURL)
	}
	if e.TokenURL != "http://localhost:8082/realms/CValidator1/protocol/openid-connect/token" {
		t.Errorf("TokenURL = %q", e.TokenURL)
	}
}

func TestEndpointDiscovery_GlobalAudienceAndScopeApplyWhenSlotUnset(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_AUDIENCE":                    "https://global-audience.example.test",
		"CANTON_LOCALNET_SCOPE":                       "global_scope",
		"CANTON_LOCALNET_B_VALIDATOR_1_AUDIENCE":      "https://slot-audience.example.test",
		"CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID":     "custom-client",
		"CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET": "custom-secret",
	}
	d := NewEndpointDiscoveryWithEnv(envLookup(env))

	a, err := d.For(RoleAValidator1)
	if err != nil {
		t.Fatalf("For(a): %v", err)
	}
	if a.Audience != "https://global-audience.example.test" || a.Scope != "global_scope" {
		t.Errorf("a audience/scope = %q/%q, want the global values", a.Audience, a.Scope)
	}

	b, err := d.For(RoleBValidator1)
	if err != nil {
		t.Fatalf("For(b): %v", err)
	}
	if b.Audience != "https://slot-audience.example.test" || b.Scope != "global_scope" {
		t.Errorf("b audience/scope = %q/%q, want slot audience and global scope", b.Audience, b.Scope)
	}
	if b.ClientID != "custom-client" || b.ClientSecret != "custom-secret" {
		t.Errorf("b credentials = %q/%q", b.ClientID, b.ClientSecret)
	}
}

func TestEndpointDiscovery_UnknownRole(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(envLookup(nil))
	if _, err := d.For(Role("nope")); err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestEndpointDiscovery_RoleSvValidator1NamesEveryMissingVariable(t *testing.T) {
	d := NewEndpointDiscoveryWithEnv(envLookup(nil))
	_, err := d.For(RoleSvValidator1)
	if err == nil {
		t.Fatal("expected error when no sv variable is set")
	}
	for _, name := range []string{
		"CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL",
		"CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID",
		"CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("err = %q, want it to mention %s", err.Error(), name)
		}
	}
}

func TestEndpointDiscovery_RoleSvValidator1DoesNotComposeARealmFromKeycloakHost(t *testing.T) {
	env := map[string]string{
		"CANTON_LOCALNET_KEYCLOAK_HOST":                "auth.example.com",
		"CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID":     "sv-client",
		"CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET": "sv-secret",
	}
	_, err := NewEndpointDiscoveryWithEnv(envLookup(env)).For(RoleSvValidator1)
	if err == nil {
		t.Fatal("expected error: the stack ships no sv realm to derive a token URL from")
	}
	if !strings.Contains(err.Error(), "CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL") {
		t.Errorf("err = %q, want it to mention the token url variable", err.Error())
	}
}
