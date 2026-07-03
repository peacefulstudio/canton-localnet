// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package yamlconfig

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDefaultsAllSlotsOn(t *testing.T) {
	t.Parallel()
	cfg := Defaults()
	if !cfg.Modules.Obs || !cfg.Modules.Pqs {
		t.Errorf("default modules: want obs=true pqs=true, got %+v", cfg.Modules)
	}
	for _, slot := range KnownSlots {
		v, ok := cfg.Validators[slot]
		if !ok {
			t.Errorf("default Validators missing slot %q", slot)
			continue
		}
		if !v.Enabled {
			t.Errorf("slot %q: expected default Enabled=true", slot)
		}
		if v.PartyHint != slot {
			t.Errorf("slot %q: expected default PartyHint=%q, got %q", slot, slot, v.PartyHint)
		}
	}
}

func TestDefaultsMultiSyncOff(t *testing.T) {
	t.Parallel()
	if Defaults().MultiSync {
		t.Errorf("Defaults().MultiSync = true, want false")
	}
}

func TestParseMultiSync(t *testing.T) {
	t.Parallel()
	data := []byte("schemaVersion: preview-1\nmultiSync: true\n")
	cfg, err := Parse(data, "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.MultiSync {
		t.Errorf("MultiSync = false, want true")
	}
}

func TestDiscoverWalksUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, FileName)
	if err := os.WriteFile(configPath, []byte("schemaVersion: preview-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(deep)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != configPath {
		t.Errorf("Discover: want %q, got %q", configPath, got)
	}
}

func TestDiscoverReturnsEmptyWhenAbsent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	isolated := filepath.Join(root, "no-config-here")
	if err := os.MkdirAll(isolated, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(isolated)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != "" {
		t.Errorf("Discover: expected empty path when no config in any ancestor, got %q", got)
	}
}

func TestDiscoverPrefersNearestAncestor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "outer", "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	outerCfg := filepath.Join(root, "outer", FileName)
	rootCfg := filepath.Join(root, FileName)
	for _, p := range []string{outerCfg, rootCfg} {
		if err := os.WriteFile(p, []byte("schemaVersion: preview-1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Discover(nested)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != outerCfg {
		t.Errorf("Discover: expected nearest ancestor %q, got %q", outerCfg, got)
	}
}

func TestParseMergesOverDefaults(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
modules:
  obs: false
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
    auth:
      clientId: app-provider-validator
      clientSecret: shhh
    parties:
      - name: featuredapp
        primary: true
  c-validator-1:
    enabled: false
`)
	cfg, err := Parse(data, "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Modules.Obs {
		t.Errorf("obs override not applied: %+v", cfg.Modules)
	}
	if !cfg.Modules.Pqs {
		t.Errorf("pqs default lost: %+v", cfg.Modules)
	}
	a := cfg.Validators["a-validator-1"]
	if a.PartyHint != "featuredapp-validator-1" {
		t.Errorf("a partyHint: got %q", a.PartyHint)
	}
	if a.Auth.ClientID != "app-provider-validator" || a.Auth.ClientSecret != "shhh" {
		t.Errorf("a auth: got %+v", a.Auth)
	}
	if len(a.Parties) != 1 || a.Parties[0].Name != "featuredapp" || !a.Parties[0].Primary {
		t.Errorf("a parties: got %+v", a.Parties)
	}
	c := cfg.Validators["c-validator-1"]
	if c.Enabled {
		t.Errorf("c-validator-1: expected Enabled=false")
	}
	b := cfg.Validators["b-validator-1"]
	if !b.Enabled || b.PartyHint != "b-validator-1" {
		t.Errorf("b-validator-1 defaults lost: %+v", b)
	}
}

func TestParseRejectsUnknownSlot(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
validators:
  e-validator-1: { enabled: true }
`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for unknown slot, got nil")
	}
	if !strings.Contains(err.Error(), "unknown validator slot") {
		t.Errorf("error message should mention unknown slot, got: %v", err)
	}
	if !strings.Contains(err.Error(), "e-validator-1") {
		t.Errorf("error message should mention the offending slot name, got: %v", err)
	}
}

func TestParseRejectsMissingSchemaVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`modules: { obs: true }`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for missing schemaVersion, got nil")
	}
	if !strings.Contains(err.Error(), "schemaVersion") {
		t.Errorf("error should mention schemaVersion, got: %v", err)
	}
}

func TestParseRejectsSchemaVersionMismatch(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: v2`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for schema mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported schemaVersion") {
		t.Errorf("error should mention unsupported schemaVersion, got: %v", err)
	}
}

func TestParseRejectsUnknownTopLevelField(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
nonsense: true
`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestParseRejectsDisablingSV(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
validators:
  sv-validator-1: { enabled: false }
`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for SV disable, got nil")
	}
	if !strings.Contains(err.Error(), "non-toggleable") {
		t.Errorf("error should mention non-toggleable SV, got: %v", err)
	}
}

func TestParseResolvesEnvVarInSecret(t *testing.T) {
	t.Setenv("CLN_TEST_SECRET_FOO", "from-env")
	data := []byte(`schemaVersion: preview-1
validators:
  a-validator-1:
    auth:
      clientSecret: ${CLN_TEST_SECRET_FOO}
`)
	cfg, err := Parse(data, "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Validators["a-validator-1"].Auth.ClientSecret; got != "from-env" {
		t.Errorf("clientSecret env resolution: got %q", got)
	}
}

func TestParseUnresolvedEnvVarFails(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
validators:
  a-validator-1:
    auth:
      clientSecret: ${CLN_TEST_DEFINITELY_NOT_SET_xyz}
`)
	_, err := Parse(data, "test.yaml")
	if err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestEnvEmissionOnDefaults(t *testing.T) {
	t.Parallel()
	env := Defaults().Env()
	want := map[string]string{
		"OBS_PROFILE":               "on",
		"PQS_PROFILE":               "on",
		"SV_VALIDATOR_1_PROFILE":    "on",
		"A_VALIDATOR_1_PROFILE":     "on",
		"B_VALIDATOR_1_PROFILE":     "on",
		"C_VALIDATOR_1_PROFILE":     "on",
		"D_VALIDATOR_1_PROFILE":     "on",
		"SV_VALIDATOR_1_PARTY_HINT": "sv-validator-1",
		"A_VALIDATOR_1_PARTY_HINT":  "a-validator-1",
		"B_VALIDATOR_1_PARTY_HINT":  "b-validator-1",
		"C_VALIDATOR_1_PARTY_HINT":  "c-validator-1",
		"D_VALIDATOR_1_PARTY_HINT":  "d-validator-1",
	}
	assertEnvContains(t, env, want)
	if !sort.StringsAreSorted(env) {
		t.Errorf("Env output must be sorted, got %v", env)
	}
}

func TestEnvEmissionForRepresentativeConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`schemaVersion: preview-1
modules:
  obs: false
  pqs: true
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
    auth:
      clientId: app-provider-validator
      clientSecret: literal-secret
  b-validator-1:
    partyHint: alice-validator-1
  c-validator-1:
    enabled: false
  d-validator-1:
    enabled: false
`)
	cfg, err := Parse(data, "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	env := cfg.Env()
	want := map[string]string{
		"OBS_PROFILE":                       "off",
		"PQS_PROFILE":                       "on",
		"SV_VALIDATOR_1_PROFILE":            "on",
		"A_VALIDATOR_1_PROFILE":             "on",
		"B_VALIDATOR_1_PROFILE":             "on",
		"C_VALIDATOR_1_PROFILE":             "off",
		"D_VALIDATOR_1_PROFILE":             "off",
		"A_VALIDATOR_1_PARTY_HINT":          "featuredapp-validator-1",
		"B_VALIDATOR_1_PARTY_HINT":          "alice-validator-1",
		"A_VALIDATOR_1_OAUTH_CLIENT_ID":     "app-provider-validator",
		"A_VALIDATOR_1_OAUTH_CLIENT_SECRET": "literal-secret",
	}
	assertEnvContains(t, env, want)
	slots := cfg.EnabledSlots()
	wantSlots := []string{"sv-validator-1", "a-validator-1", "b-validator-1"}
	if !equalStringSlice(slots, wantSlots) {
		t.Errorf("EnabledSlots: want %v, got %v", wantSlots, slots)
	}
}

func TestResolveExplicitPathWins(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	discoverable := filepath.Join(root, FileName)
	if err := os.WriteFile(discoverable, []byte("schemaVersion: preview-1\nmodules: { obs: true }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(root, "alt.yaml")
	if err := os.WriteFile(explicit, []byte("schemaVersion: preview-1\nmodules: { obs: false }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, used, err := Resolve(explicit, root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if used != explicit {
		t.Errorf("Resolve: want path %q, got %q", explicit, used)
	}
	if cfg.Modules.Obs {
		t.Errorf("Resolve: explicit path should win, obs should be off")
	}
}

func TestResolveFallsBackToDefaultsWhenNotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	isolated := filepath.Join(root, "empty")
	if err := os.MkdirAll(isolated, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, used, err := Resolve("", isolated)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if used != "" {
		t.Errorf("Resolve: expected empty source path on fallback, got %q", used)
	}
	if !cfg.Modules.Obs || !cfg.Modules.Pqs {
		t.Errorf("Resolve fallback should yield defaults, got %+v", cfg.Modules)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func assertEnvContains(t *testing.T, env []string, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, kv := range env {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			t.Errorf("malformed env entry %q", kv)
			continue
		}
		got[kv[:idx]] = kv[idx+1:]
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
