// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"path/filepath"
	"testing"
)

func TestOAuth2SlotsDefaultTheValidatorUserIDToTheComposeEnvFile(t *testing.T) {
	root := repoRoot(t)
	for _, s := range All() {
		if s.AuthKind != AuthKindOAuth2 {
			continue
		}
		path := filepath.Join(root, "compose", "modules", "keycloak", "env", s.Canonical, "on", "oauth2.env")
		composeEnv, err := readEnvFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		want := composeEnv["AUTH_"+s.EnvPrefix()+"_VALIDATOR_USER_ID"]
		if s.ValidatorUserID != want {
			t.Errorf("%s: built-in ValidatorUserID = %q, want %q from %s", s.Canonical, s.ValidatorUserID, want, path)
		}
	}
}

func TestResolveFallsBackToTheHS256SubjectForSV(t *testing.T) {
	s, err := Parse("sv")
	if err != nil {
		t.Fatal(err)
	}

	ep, err := Resolve(s, "", func(string) (string, bool) { return "", false })

	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ep.ValidatorUserID != ep.HS256User {
		t.Errorf("validator user id = %q, want the HS256 subject %q", ep.ValidatorUserID, ep.HS256User)
	}
}

func TestResolvePrefersTheUserIDEnvOverride(t *testing.T) {
	s, err := Parse("a")
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) {
		if key == "CANTON_LOCALNET_A_VALIDATOR_1_USER_ID" {
			return "override-user", true
		}
		return "", false
	}

	ep, err := Resolve(s, "", lookup)

	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ep.ValidatorUserID != "override-user" {
		t.Errorf("validator user id = %q, want the env override", ep.ValidatorUserID)
	}
}
