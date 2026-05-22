// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthTokenOAuth2HitsRealmTokenEndpoint(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "dev-secret")

	var capturedPath string
	var capturedForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		capturedForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"jwt-here","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)

	stdout := captureRoot(t, "auth", "token", "--slot", "a")

	if got := strings.TrimSpace(stdout); got != "jwt-here" {
		t.Errorf("stdout: got %q, want jwt-here", got)
	}
	if capturedPath != "/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("path: got %q", capturedPath)
	}
	if capturedForm.Get("client_id") != "a-validator-1-validator" {
		t.Errorf("client_id: got %q", capturedForm.Get("client_id"))
	}
	if capturedForm.Get("client_secret") != "dev-secret" {
		t.Errorf("client_secret: got %q", capturedForm.Get("client_secret"))
	}
	if capturedForm.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type: got %q", capturedForm.Get("grant_type"))
	}
}

func TestAuthTokenAcceptsShortSlotAlias(t *testing.T) {
	t.Setenv("CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET", "x")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)

	stdout := captureRoot(t, "auth", "token", "--slot", "b")
	if got := strings.TrimSpace(stdout); got != "t" {
		t.Errorf("stdout: got %q", got)
	}
}

func TestAuthTokenSVMintsSelfSignedJWT(t *testing.T) {
	stdout := captureRoot(t, "auth", "token", "--slot", "sv")
	tok := strings.TrimSpace(stdout)
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected JWT with 3 parts, got %d in %q", len(parts), tok)
	}
	mac := hmac.New(sha256.New, []byte("unsafe"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if !hmac.Equal(mac.Sum(nil), sig) {
		t.Error("HS256 signature did not verify against shared secret 'unsafe'")
	}
}

func TestAuthTokenRejectsUnknownSlot(t *testing.T) {
	err := executeRootError(t, "auth", "token", "--slot", "e")
	if err == nil {
		t.Fatal("expected error for unknown slot")
	}
	if !strings.Contains(err.Error(), "unknown slot") {
		t.Errorf("expected unknown-slot error, got %q", err.Error())
	}
}

func TestAuthTokenReadsClientCredentialsFromYAML(t *testing.T) {
	root := t.TempDir()
	yamlPath := root + "/canton-localnet.yaml"
	body := "schemaVersion: preview-1\n" +
		"validators:\n" +
		"  a-validator-1:\n" +
		"    auth:\n" +
		"      clientId: yaml-client\n" +
		"      clientSecret: yaml-secret\n"
	if err := os.WriteFile(yamlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)

	captureRoot(t, "auth", "token", "--slot", "a", "--config", yamlPath)

	if got.Get("client_id") != "yaml-client" {
		t.Errorf("client_id should come from YAML, got %q", got.Get("client_id"))
	}
	if got.Get("client_secret") != "yaml-secret" {
		t.Errorf("client_secret should come from YAML, got %q", got.Get("client_secret"))
	}
}

func TestAuthTokenEnvVarBeatsYAMLClientSecret(t *testing.T) {
	root := t.TempDir()
	yamlPath := root + "/canton-localnet.yaml"
	body := "schemaVersion: preview-1\n" +
		"validators:\n" +
		"  a-validator-1:\n" +
		"    auth:\n" +
		"      clientSecret: yaml-secret\n"
	if err := os.WriteFile(yamlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "env-secret")

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)

	captureRoot(t, "auth", "token", "--slot", "a", "--config", yamlPath)

	if got.Get("client_secret") != "env-secret" {
		t.Errorf("env var must beat YAML secret, got %q", got.Get("client_secret"))
	}
}

func TestInfoEnvBeatsYAMLPartyHint(t *testing.T) {
	root := t.TempDir()
	yamlPath := root + "/canton-localnet.yaml"
	body := "schemaVersion: preview-1\nvalidators:\n  a-validator-1:\n    partyHint: yaml-hint\n"
	if err := os.WriteFile(yamlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "x")
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_PARTY_HINT", "env-hint")

	stdout := captureRoot(t, "info", "--slot", "a", "--json", "--offline", "--config", yamlPath)
	var got slotInfo
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if got.PartyHint != "env-hint" {
		t.Errorf("env var must beat YAML party_hint, got %q", got.PartyHint)
	}
}

func TestAuthTokenEmptyEnvVarFallsThroughToYAML(t *testing.T) {
	root := t.TempDir()
	yamlPath := root + "/canton-localnet.yaml"
	body := "schemaVersion: preview-1\nvalidators:\n  a-validator-1:\n    auth:\n      clientSecret: yaml-secret\n"
	if err := os.WriteFile(yamlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	// Explicit-empty env var must be treated as unset, matching slot.Resolve's
	// envOr semantics — otherwise the documented precedence breaks.
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "   ")

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)

	captureRoot(t, "auth", "token", "--slot", "a", "--config", yamlPath)
	if got.Get("client_secret") != "yaml-secret" {
		t.Errorf("empty-env should fall through to YAML, got %q", got.Get("client_secret"))
	}
}

func TestAuthTokenReadsSecretFromRepoEnvFile(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "compose", "modules", "keycloak", "env", "c-validator-1", "on")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "AUTH_C_VALIDATOR_1_VALIDATOR_CLIENT_ID=c-validator-1-validator\n" +
		"AUTH_C_VALIDATOR_1_VALIDATOR_CLIENT_SECRET=from-disk\n"
	if err := os.WriteFile(filepath.Join(envDir, "oauth2.env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		if form.Get("client_secret") != "from-disk" {
			t.Errorf("client_secret: got %q, want from-disk", form.Get("client_secret"))
		}
		_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.URL)
	t.Setenv("CANTON_LOCALNET_HOST", host)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", port)
	t.Setenv("CANTON_LOCALNET_C_VALIDATOR_1_CLIENT_SECRET", "")

	stdout := captureRoot(t, "auth", "token", "--slot", "c", "--repo-root", root)
	if strings.TrimSpace(stdout) != "t" {
		t.Errorf("stdout: got %q", stdout)
	}
}

func splitHostPort(t *testing.T, raw string) (string, string) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Hostname()
	port := u.Port()
	if host == "" || port == "" {
		t.Fatalf("invalid server URL %q", raw)
	}
	return host, port
}

func captureRoot(t *testing.T, args ...string) string {
	t.Helper()
	stdout := &bytes.Buffer{}
	_, factory := newFakeRunnerFactory()
	root := newRootCommand(factory)
	root.SetOut(stdout)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return stdout.String()
}

func executeRootError(t *testing.T, args ...string) error {
	t.Helper()
	_, factory := newFakeRunnerFactory()
	root := newRootCommand(factory)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}
