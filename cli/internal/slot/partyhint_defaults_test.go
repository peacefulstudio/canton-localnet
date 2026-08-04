// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	return root
}

func envFileValue(t *testing.T, path, key string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("%s: no %s= line found", path, key)
	return ""
}

func partyHintSlots() []Slot {
	out := []Slot{}
	for _, s := range All() {
		if s.Canonical != "sv-validator-1" {
			out = append(out, s)
		}
	}
	return out
}

func TestModuleEnvFilesDefaultPartyHintToSlotName(t *testing.T) {
	root := repoRoot(t)
	for _, s := range partyHintSlots() {
		key := s.EnvPrefix() + "_PARTY_HINT"
		want := fmt.Sprintf("${%s:-%s}", key, s.Canonical)
		paths := []string{
			filepath.Join(root, "compose", "modules", "localnet", "env", s.Canonical+"-auth-on.env"),
			filepath.Join(root, "compose", "modules", "keycloak", "env", s.Canonical, "on", "oauth2.env"),
		}
		for _, path := range paths {
			if got := envFileValue(t, path, key); got != want {
				t.Errorf("%s: %s = %q, want %q", path, key, got, want)
			}
		}
	}
}

func TestEnvDefaultsDeclaresNoSlotPartyHints(t *testing.T) {
	path := filepath.Join(repoRoot(t), "compose", ".env.defaults")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "_VALIDATOR_1_PARTY_HINT=") {
			t.Errorf("%s:%d: slot party hints belong in canton-localnet.yaml, not .env.defaults: %s", path, i+1, trimmed)
		}
	}
}
