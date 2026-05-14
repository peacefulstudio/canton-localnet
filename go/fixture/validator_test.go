// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFixture_ValidatorRequiresSetup(t *testing.T) {
	f, err := New(Config{
		Role:      RoleAValidator1,
		Discovery: NewEndpointDiscoveryWithEnv(func(string) string { return "" }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := f.Validator(RoleBValidator1); err == nil {
		t.Fatal("expected error when Validator is called before Setup")
	} else if !strings.Contains(err.Error(), "Setup") {
		t.Errorf("err = %q, want it to mention Setup", err.Error())
	}
}

func TestFixture_ValidatorForDefaultRoleReturnsSharedClients(t *testing.T) {
	ledger, keycloak, ledgerHost, ledgerPort, keycloakHost, keycloakPort := newStubStack(t)
	t.Cleanup(ledger.Close)
	t.Cleanup(keycloak.Close)

	env := map[string]string{
		"CANTON_LOCALNET_HOST":                        ledgerHost,
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":     ledgerPort,
		"CANTON_LOCALNET_KEYCLOAK_HOST":               keycloakHost,
		"CANTON_LOCALNET_KEYCLOAK_PORT":               keycloakPort,
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "cid",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "csecret",
	}
	f := newSetupFixture(t, RoleAValidator1, env)

	v, err := f.Validator(RoleAValidator1)
	if err != nil {
		t.Fatalf("Validator(default): %v", err)
	}
	if v.Admin() != f.Admin() {
		t.Error("expected default-role view to share Admin with Fixture")
	}
	if v.PartyAllocator() != f.PartyAllocator() {
		t.Error("expected default-role view to share PartyAllocator with Fixture")
	}
	if v.Role() != RoleAValidator1 {
		t.Errorf("Role = %q", v.Role())
	}
}

func TestFixture_ValidatorForOtherRoleBuildsScopedClients(t *testing.T) {
	ledger, keycloak, ledgerHost, ledgerPort, keycloakHost, keycloakPort := newStubStack(t)
	t.Cleanup(ledger.Close)
	t.Cleanup(keycloak.Close)

	env := map[string]string{
		"CANTON_LOCALNET_HOST":                        ledgerHost,
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":     ledgerPort,
		"CANTON_LOCALNET_B_VALIDATOR_1_JSON_PORT":     ledgerPort,
		"CANTON_LOCALNET_KEYCLOAK_HOST":               keycloakHost,
		"CANTON_LOCALNET_KEYCLOAK_PORT":               keycloakPort,
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "cid-a",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "csecret-a",
		"CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID":     "cid-b",
		"CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET": "csecret-b",
	}
	f := newSetupFixture(t, RoleAValidator1, env)

	a, err := f.Validator(RoleAValidator1)
	if err != nil {
		t.Fatalf("Validator(A): %v", err)
	}
	b, err := f.Validator(RoleBValidator1)
	if err != nil {
		t.Fatalf("Validator(B): %v", err)
	}
	if a.Admin() == b.Admin() {
		t.Error("expected Admin clients for different roles to differ")
	}
	if a.Endpoints().ClientID == b.Endpoints().ClientID {
		t.Errorf("expected distinct ClientIDs, got %q == %q", a.Endpoints().ClientID, b.Endpoints().ClientID)
	}
	if b.Endpoints().ClientID != "cid-b" {
		t.Errorf("Validator(B) ClientID = %q, want cid-b", b.Endpoints().ClientID)
	}
	if !strings.Contains(b.Endpoints().TokenURL, "/realms/BValidator1/") {
		t.Errorf("Validator(B) TokenURL = %q, want it to point at BValidator1 realm", b.Endpoints().TokenURL)
	}

	b2, err := f.Validator(RoleBValidator1)
	if err != nil {
		t.Fatalf("Validator(B) second call: %v", err)
	}
	if b2 != b {
		t.Error("expected Validator() to return cached instance on repeated calls")
	}
}

func TestFixture_ValidatorUnknownRoleReturnsError(t *testing.T) {
	f := newSetupFixture(t, RoleAValidator1, minimalStubEnv(t))
	if _, err := f.Validator(Role("nope-validator-1")); err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestFixture_MustValidatorPanicsOnUnknownRole(t *testing.T) {
	f := newSetupFixture(t, RoleAValidator1, minimalStubEnv(t))
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustValidator with unknown role")
		}
	}()
	_ = f.MustValidator(Role("ghost"))
}

func TestKnownRoles_StableOrder(t *testing.T) {
	got := KnownRoles()
	want := []Role{RoleSvValidator1, RoleAValidator1, RoleBValidator1, RoleCValidator1, RoleDValidator1}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownRoles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func newStubStack(t *testing.T) (ledger, keycloak *httptest.Server, ledgerHost, ledgerPort, keycloakHost, keycloakPort string) {
	t.Helper()
	ledger = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::stub"}`)
	}))
	keycloak = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-stub",
			"token_type":   "Bearer",
			"expires_in":   300,
		})
	}))
	ledgerHost, ledgerPort = splitHostPort(t, ledger.URL)
	keycloakHost, keycloakPort = splitHostPort(t, keycloak.URL)
	return
}

func newSetupFixture(t *testing.T, role Role, env map[string]string) *Fixture {
	t.Helper()
	f, err := New(Config{
		Role:      role,
		Discovery: NewEndpointDiscoveryWithEnv(func(k string) string { return env[k] }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = f.Teardown(context.Background()) })
	return f
}

func minimalStubEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "cid",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "csecret",
	}
}
