// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import "testing"

func TestResolveCIAppliesBuiltInDefaultForSlot(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	base, err := Resolve(a, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, err := ResolveCI(base, "", 1, emptyEnv)
	if err != nil {
		t.Fatalf("ResolveCI: %v", err)
	}
	if got.ClientID != "a-validator-1-ci-1" {
		t.Errorf("ClientID: got %q", got.ClientID)
	}
	if got.ClientSecret != "VSLk2bzpSPKIY8qGsQuY4uYdkdoccUG6" {
		t.Errorf("ClientSecret: got %q", got.ClientSecret)
	}
	if got.ValidatorUserID != "cfc83af9-a97e-4e5b-85a1-b0c4cc49323c" {
		t.Errorf("ValidatorUserID: got %q", got.ValidatorUserID)
	}
}

func TestResolveCIDiffersPerSlot(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	base, err := Resolve(a, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	one, err := ResolveCI(base, "", 1, emptyEnv)
	if err != nil {
		t.Fatalf("ResolveCI(1): %v", err)
	}
	two, err := ResolveCI(base, "", 2, emptyEnv)
	if err != nil {
		t.Fatalf("ResolveCI(2): %v", err)
	}
	if one.ClientID == two.ClientID {
		t.Errorf("expected distinct ClientID per CI slot, both %q", one.ClientID)
	}
	if one.ValidatorUserID == two.ValidatorUserID {
		t.Errorf("expected distinct ValidatorUserID per CI slot, both %q", one.ValidatorUserID)
	}
}

func TestResolveCIReadsComposeEnvFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_CI_1_CLIENT_SECRET=rotated-ci-secret-from-file\n")

	a, _ := Parse("a")
	base, err := Resolve(a, root, emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, err := ResolveCI(base, root, 1, emptyEnv)
	if err != nil {
		t.Fatalf("ResolveCI: %v", err)
	}
	if got.ClientSecret != "rotated-ci-secret-from-file" {
		t.Errorf("ClientSecret: got %q", got.ClientSecret)
	}
}

func TestResolveCIEnvOverridesComposeFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeComposeEnvFile(t, root, "a-validator-1", "AUTH_A_VALIDATOR_1_CI_1_CLIENT_SECRET=file-secret\n")

	a, _ := Parse("a")
	base, err := Resolve(a, root, emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, err := ResolveCI(base, root, 1, envFrom(map[string]string{
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "env-secret",
	}))
	if err != nil {
		t.Fatalf("ResolveCI: %v", err)
	}
	if got.ClientSecret != "env-secret" {
		t.Errorf("expected env override to win, got %q", got.ClientSecret)
	}
}

func TestResolveCIRejectsNonAValidatorSlot(t *testing.T) {
	t.Parallel()
	b, _ := Parse("b")
	base, err := Resolve(b, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := ResolveCI(base, "", 1, emptyEnv); err == nil {
		t.Error("expected an error for a non-a-validator-1 slot, got nil")
	}
}

func TestResolveCIRejectsOutOfRangeSlot(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	base, err := Resolve(a, "", emptyEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, ciSlot := range []int{0, 5, -1} {
		if _, err := ResolveCI(base, "", ciSlot, emptyEnv); err == nil {
			t.Errorf("expected an error for --ci-slot %d, got nil", ciSlot)
		}
	}
}
