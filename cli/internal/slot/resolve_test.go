// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func emptyEnv(string) (string, bool) { return "", false }

func envFrom(m map[string]string) EnvLookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func writeComposeEnvFile(t *testing.T, root, slot, body string) {
	t.Helper()
	dir := filepath.Join(root, "compose", "modules", "keycloak", "env", slot, "on")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "oauth2.env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOAuth2ReadsComposeEnvFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_ID=a-validator-1-validator\nAUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_SECRET=rotated-secret-from-file\n")

	a, _ := Parse("a")
	got, err := Resolve(a, root, emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ClientID != "a-validator-1-validator" {
		t.Errorf("ClientID: got %q", got.ClientID)
	}
	if got.ClientSecret != "rotated-secret-from-file" {
		t.Errorf("ClientSecret: got %q", got.ClientSecret)
	}
	if got.TokenURLHost != "http://localhost:8082/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("TokenURLHost: got %q", got.TokenURLHost)
	}
	if got.TokenURLInternal != "http://nginx-keycloak:8082/realms/AValidator1/protocol/openid-connect/token" {
		t.Errorf("TokenURLInternal: got %q", got.TokenURLInternal)
	}
	if got.JSONLedgerAPIURL != "http://localhost:11975" {
		t.Errorf("JSONLedgerAPIURL: got %q", got.JSONLedgerAPIURL)
	}
}

func TestResolveOAuth2EnvOverridesComposeFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_SECRET=file-secret\n")

	a, _ := Parse("a")
	got, err := Resolve(a, root, envFrom(map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "env-secret",
	}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ClientSecret != "env-secret" {
		t.Errorf("expected env override to win, got %q", got.ClientSecret)
	}
}

func TestResolveOAuth2WithoutSecretReturnsEmptyClientSecret(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	got, err := Resolve(a, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve should not require a secret (info --offline path): %v", err)
	}
	if got.ClientSecret != "" {
		t.Errorf("expected empty ClientSecret, got %q", got.ClientSecret)
	}
	if got.TokenURLHost == "" {
		t.Error("TokenURLHost should still resolve from realm")
	}
}

func TestResolveSvUsesHS256Defaults(t *testing.T) {
	t.Parallel()
	sv, _ := Parse("sv")
	got, err := Resolve(sv, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.HS256Secret != "unsafe" {
		t.Errorf("HS256Secret: got %q, want unsafe", got.HS256Secret)
	}
	if got.HS256User != "ledger-api-user" {
		t.Errorf("HS256User: got %q, want ledger-api-user", got.HS256User)
	}
	if got.TokenURLHost != "" {
		t.Errorf("sv should not have a Keycloak token URL, got %q", got.TokenURLHost)
	}
}

func TestResolveHonoursHostOverride(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_SECRET=s\n")
	got, err := Resolve(a, root, envFrom(map[string]string{
		"CANTON_LOCALNET_HOST": "vm.internal",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got.JSONLedgerAPIURL != "http://vm.internal:11975" {
		t.Errorf("JSONLedgerAPIURL: got %q", got.JSONLedgerAPIURL)
	}
	if got.LedgerGrpcURL != "vm.internal:11901" {
		t.Errorf("LedgerGrpcURL: got %q", got.LedgerGrpcURL)
	}
}

func TestReadEnvFileStripsInlineComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "oauth2.env")
	if err := os.WriteFile(path, []byte("FOO=bar  # trailing comment\nBAZ=qux\n# leading comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want bar", got["FOO"])
	}
	if got["BAZ"] != "qux" {
		t.Errorf("BAZ: got %q, want qux", got["BAZ"])
	}
}

func TestResolveHonoursTokenURLOverride(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	got, err := Resolve(a, "", envFrom(map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL": "https://idp.example.com/oauth2/token",
	}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TokenURLHost != "https://idp.example.com/oauth2/token" {
		t.Errorf("TokenURLHost: got %q, want https://idp.example.com/oauth2/token", got.TokenURLHost)
	}
}

func TestReadEnvFileRejectsMalformedLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "oauth2.env")
	if err := os.WriteFile(path, []byte("FOO bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadEnvFile(path)
	if err == nil {
		t.Fatal("expected error for KEY VALUE without '='")
	}
	if !strings.Contains(err.Error(), "expected KEY=VALUE") {
		t.Errorf("expected KEY=VALUE message, got %q", err.Error())
	}
}

func TestStripPairedQuotesOnlyStripsMatchingPairs(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`"foo"`:  `foo`,
		`'foo'`:  `foo`,
		`"foo'`:  `"foo'`,
		`'foo"`:  `'foo"`,
		`foo`:    `foo`,
		`"`:      `"`,
		`""`:     ``,
	}
	for in, want := range cases {
		if got := stripPairedQuotes(in); got != want {
			t.Errorf("stripPairedQuotes(%q): got %q, want %q", in, got, want)
		}
	}
}

func TestResolveSurfacesReadEnvFileError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "compose", "modules", "keycloak", "env", "a-validator-1", "on")
	if err := os.MkdirAll(filepath.Join(dir, "oauth2.env"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, _ := Parse("a")
	_, err := Resolve(a, root, emptyEnv)
	if err == nil {
		t.Fatal("expected error when oauth2.env path is a directory")
	}
}

func TestResolveHonoursKeycloakHostOverride(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	got, err := Resolve(a, "", envFrom(map[string]string{
		"CANTON_LOCALNET_KEYCLOAK_HOST": "keycloak.example.com",
	}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.KeycloakHostBase != "http://keycloak.example.com:8082" {
		t.Errorf("KeycloakHostBase: got %q, want http://keycloak.example.com:8082", got.KeycloakHostBase)
	}
	if got.Host != "localhost" {
		t.Errorf("Host should remain localhost when only KEYCLOAK_HOST is overridden, got %q", got.Host)
	}
}

func TestResolveHonoursPerSlotAudienceOverride(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	got, err := Resolve(a, "", envFrom(map[string]string{
		"CANTON_LOCALNET_AUDIENCE":             "https://global.audience",
		"CANTON_LOCALNET_A_VALIDATOR_1_AUDIENCE": "https://slot.audience",
	}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Audience != "https://slot.audience" {
		t.Errorf("Audience: got %q, want per-slot override https://slot.audience", got.Audience)
	}
}

func TestResolveHonoursPerSlotScopeOverride(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	got, err := Resolve(a, "", envFrom(map[string]string{
		"CANTON_LOCALNET_SCOPE":             "global-scope",
		"CANTON_LOCALNET_A_VALIDATOR_1_SCOPE": "slot-scope",
	}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Scope != "slot-scope" {
		t.Errorf("Scope: got %q, want per-slot override slot-scope", got.Scope)
	}
}

func TestResolveClientIDPrefersComposeFileOverDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1",
		"AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_ID=rotated-client-id\n"+
			"AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_SECRET=s\n")
	a, _ := Parse("a")
	got, err := Resolve(a, root, emptyEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != "rotated-client-id" {
		t.Errorf("ClientID: got %q, want rotated-client-id", got.ClientID)
	}
}

func TestResolveOAuth2WalletLoginDefaultsToTheSlotsOnboardedUser(t *testing.T) {
	t.Parallel()
	b, _ := Parse("b")
	got, err := Resolve(b, "", emptyEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got.WalletClientID != "b-validator-1-unsafe" || got.WalletUser != "b-validator-1" || got.WalletPassword != "abc123" {
		t.Fatalf("wallet login = %q / %q / %q, want b-validator-1-unsafe / b-validator-1 / abc123", got.WalletClientID, got.WalletUser, got.WalletPassword)
	}
}

func TestResolveOAuth2WalletLoginReadsComposeEnvFileThenEnvOverrides(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_AUTO_CONFIG_CLIENT_ID=file-client\nAUTH_A_VALIDATOR_1_WALLET_ADMIN_USER_NAME=file-user\nAUTH_A_VALIDATOR_1_WALLET_ADMIN_USER_PASSWORD=file-password\n")
	a, _ := Parse("a")

	fromFile, err := Resolve(a, root, emptyEnv)
	if err != nil {
		t.Fatal(err)
	}
	if fromFile.WalletClientID != "file-client" || fromFile.WalletUser != "file-user" || fromFile.WalletPassword != "file-password" {
		t.Fatalf("from file = %q / %q / %q", fromFile.WalletClientID, fromFile.WalletUser, fromFile.WalletPassword)
	}

	overridden, err := Resolve(a, root, envFrom(map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_WALLET_CLIENT_ID": "env-client",
		"CANTON_LOCALNET_A_VALIDATOR_1_WALLET_USER":      "env-user",
		"CANTON_LOCALNET_A_VALIDATOR_1_WALLET_PASSWORD":  "env-password",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if overridden.WalletClientID != "env-client" || overridden.WalletUser != "env-user" || overridden.WalletPassword != "env-password" {
		t.Fatalf("overridden = %q / %q / %q", overridden.WalletClientID, overridden.WalletUser, overridden.WalletPassword)
	}
}

func writeLocalnetCommonEnv(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "compose", "modules", "localnet", "env")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "common.env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePortsFollowComposePortSuffixes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		slot               string
		commonEnv          string
		env                map[string]string
		wantJSONURL        string
		wantLedgerGrpcURL  string
		wantLedgerGrpcPort string
		wantAdminGrpcURL   string
		wantValidatorURL   string
	}{
		{
			name:               "no common.env and no overrides keeps the built-in suffixes",
			slot:               "a",
			wantJSONURL:        "http://localhost:11975",
			wantLedgerGrpcURL:  "localhost:11901",
			wantLedgerGrpcPort: "11901",
			wantAdminGrpcURL:   "localhost:11902",
			wantValidatorURL:   "localhost:11903",
		},
		{
			name: "common.env defaults replace the built-in suffixes",
			slot: "b",
			commonEnv: "PARTICIPANT_LEDGER_API_PORT_SUFFIX=${PARTICIPANT_LEDGER_API_PORT_SUFFIX:-801}\n" +
				"PARTICIPANT_ADMIN_API_PORT_SUFFIX=${PARTICIPANT_ADMIN_API_PORT_SUFFIX:-802}\n" +
				"PARTICIPANT_JSON_API_PORT_SUFFIX=${PARTICIPANT_JSON_API_PORT_SUFFIX:-875}\n" +
				"VALIDATOR_ADMIN_API_PORT_SUFFIX=${VALIDATOR_ADMIN_API_PORT_SUFFIX:-803}\n",
			wantJSONURL:        "http://localhost:12875",
			wantLedgerGrpcURL:  "localhost:12801",
			wantLedgerGrpcPort: "12801",
			wantAdminGrpcURL:   "localhost:12802",
			wantValidatorURL:   "localhost:12803",
		},
		{
			name: "process env overrides beat the common.env defaults",
			slot: "a",
			commonEnv: "PARTICIPANT_LEDGER_API_PORT_SUFFIX=${PARTICIPANT_LEDGER_API_PORT_SUFFIX:-801}\n" +
				"PARTICIPANT_JSON_API_PORT_SUFFIX=${PARTICIPANT_JSON_API_PORT_SUFFIX:-875}\n",
			env: map[string]string{
				"PARTICIPANT_LEDGER_API_PORT_SUFFIX": "911",
				"PARTICIPANT_ADMIN_API_PORT_SUFFIX":  "912",
				"PARTICIPANT_JSON_API_PORT_SUFFIX":   "976",
				"VALIDATOR_ADMIN_API_PORT_SUFFIX":    "913",
			},
			wantJSONURL:        "http://localhost:11976",
			wantLedgerGrpcURL:  "localhost:11911",
			wantLedgerGrpcPort: "11911",
			wantAdminGrpcURL:   "localhost:11912",
			wantValidatorURL:   "localhost:11913",
		},
		{
			name:               "process env overrides apply without a repo checkout",
			slot:               "sv",
			env:                map[string]string{"PARTICIPANT_JSON_API_PORT_SUFFIX": "976", "PARTICIPANT_LEDGER_API_PORT_SUFFIX": "911"},
			wantJSONURL:        "http://localhost:10976",
			wantLedgerGrpcURL:  "localhost:10911",
			wantLedgerGrpcPort: "10911",
			wantAdminGrpcURL:   "localhost:10902",
			wantValidatorURL:   "localhost:10903",
		},
		{
			name: "a per-slot JSON port override still beats the suffix",
			slot: "a",
			env: map[string]string{
				"PARTICIPANT_JSON_API_PORT_SUFFIX":        "976",
				"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT": "18080",
			},
			wantJSONURL:        "http://localhost:18080",
			wantLedgerGrpcURL:  "localhost:11901",
			wantLedgerGrpcPort: "11901",
			wantAdminGrpcURL:   "localhost:11902",
			wantValidatorURL:   "localhost:11903",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if tc.commonEnv != "" {
				writeLocalnetCommonEnv(t, root, tc.commonEnv)
			}
			s, err := Parse(tc.slot)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Resolve(s, root, envFrom(tc.env))
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.JSONLedgerAPIURL != tc.wantJSONURL {
				t.Errorf("JSONLedgerAPIURL: got %q, want %q", got.JSONLedgerAPIURL, tc.wantJSONURL)
			}
			if got.LedgerGrpcURL != tc.wantLedgerGrpcURL {
				t.Errorf("LedgerGrpcURL: got %q, want %q", got.LedgerGrpcURL, tc.wantLedgerGrpcURL)
			}
			if got.LedgerGrpcPort != tc.wantLedgerGrpcPort {
				t.Errorf("LedgerGrpcPort: got %q, want %q", got.LedgerGrpcPort, tc.wantLedgerGrpcPort)
			}
			if got.AdminGrpcURL != tc.wantAdminGrpcURL {
				t.Errorf("AdminGrpcURL: got %q, want %q", got.AdminGrpcURL, tc.wantAdminGrpcURL)
			}
			if got.ValidatorAdminURL != tc.wantValidatorURL {
				t.Errorf("ValidatorAdminURL: got %q, want %q", got.ValidatorAdminURL, tc.wantValidatorURL)
			}
		})
	}
}

func TestResolveSurfacesUnreadableLocalnetCommonEnv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLocalnetCommonEnv(t, root, "not a key value line\n")
	a, _ := Parse("a")
	if _, err := Resolve(a, root, emptyEnv); err == nil {
		t.Fatal("expected an error for a malformed common.env")
	}
}
