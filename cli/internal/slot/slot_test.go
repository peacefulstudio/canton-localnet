// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"strings"
	"testing"
)

func TestParseAcceptsShortAndCanonicalForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"a", "a-validator-1"},
		{"A", "a-validator-1"},
		{"sv", "sv-validator-1"},
		{"SV", "sv-validator-1"},
		{"a-validator-1", "a-validator-1"},
		{"A_VALIDATOR_1", "a-validator-1"},
		{" d ", "d-validator-1"},
	}
	for _, tc := range cases {
		got, err := Parse(tc.input)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.input, err)
			continue
		}
		if got.Canonical != tc.want {
			t.Errorf("Parse(%q): got %q, want %q", tc.input, got.Canonical, tc.want)
		}
	}
}

func TestParseRejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := Parse("e")
	if err == nil {
		t.Fatal("expected error for unknown slot")
	}
	if !strings.Contains(err.Error(), "unknown slot") {
		t.Errorf("expected unknown-slot message, got %q", err.Error())
	}
}

func TestSlotPorts(t *testing.T) {
	t.Parallel()
	a, err := Parse("a")
	if err != nil {
		t.Fatal(err)
	}
	if a.JSONLedgerPort() != "11975" {
		t.Errorf("JSON port for a: got %q, want 11975", a.JSONLedgerPort())
	}
	if a.LedgerGrpcPort() != "11901" {
		t.Errorf("ledger gRPC port for a: got %q, want 11901", a.LedgerGrpcPort())
	}
	if a.AdminGrpcPort() != "11902" {
		t.Errorf("admin gRPC port for a: got %q, want 11902", a.AdminGrpcPort())
	}
	if a.ValidatorAdminPort() != "11903" {
		t.Errorf("validator admin port for a: got %q, want 11903", a.ValidatorAdminPort())
	}
}

func TestAuthKindMatchesRealmShape(t *testing.T) {
	t.Parallel()
	for _, s := range All() {
		switch s.Canonical {
		case "sv-validator-1":
			if s.AuthKind != AuthKindHS256 {
				t.Errorf("sv: expected HS256, got %q", s.AuthKind)
			}
			if s.Realm != "" {
				t.Errorf("sv: expected empty realm (no Keycloak), got %q", s.Realm)
			}
		default:
			if s.AuthKind != AuthKindOAuth2 {
				t.Errorf("%s: expected OAuth2, got %q", s.Canonical, s.AuthKind)
			}
			if s.Realm == "" {
				t.Errorf("%s: expected non-empty realm", s.Canonical)
			}
		}
	}
}

func TestEnvPrefixIsScreamingSnake(t *testing.T) {
	t.Parallel()
	a, _ := Parse("a")
	if a.EnvPrefix() != "A_VALIDATOR_1" {
		t.Errorf("env prefix for a: got %q, want A_VALIDATOR_1", a.EnvPrefix())
	}
}

func TestCanonicalsListsEverySlotWithSVFirst(t *testing.T) {
	t.Parallel()
	want := []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1", "d-validator-1"}
	got := Canonicals()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Canonicals() = %v, want %v", got, want)
	}
}

func TestSVCanonicalIsTheSuperValidatorSlot(t *testing.T) {
	t.Parallel()
	if SVCanonical != "sv-validator-1" {
		t.Errorf("SVCanonical = %q, want %q", SVCanonical, "sv-validator-1")
	}
}

func TestPqsSlotsAreSvABAndC(t *testing.T) {
	t.Parallel()
	var got []string
	for _, s := range PqsSlots() {
		got = append(got, s.Canonical)
	}
	want := []string{"sv-validator-1", "a-validator-1", "b-validator-1", "c-validator-1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("PqsSlots: got %v, want %v", got, want)
	}
}

func TestPqsDefaultSlotsAreABAndC(t *testing.T) {
	t.Parallel()
	var got []string
	for _, s := range PqsDefaultSlots() {
		got = append(got, s.Canonical)
	}
	want := []string{"a-validator-1", "b-validator-1", "c-validator-1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("PqsDefaultSlots: got %v, want %v", got, want)
	}
}
