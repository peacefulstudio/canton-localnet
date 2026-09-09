// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package rights

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const validatorPrefix = "a-validator-1::"

func decodeRights(t *testing.T, entries ...string) []Right {
	t.Helper()
	var held []Right
	if err := json.Unmarshal([]byte("["+strings.Join(entries, ",")+"]"), &held); err != nil {
		t.Fatalf("decoding rights: %v", err)
	}
	return held
}

func actAs(party string) string {
	return `{"kind":{"CanActAs":{"value":{"party":"` + party + `"}}}}`
}

func readAs(party string) string {
	return `{"kind":{"CanReadAs":{"value":{"party":"` + party + `"}}}}`
}

const adminRight = `{"kind":{"ParticipantAdmin":{"value":{}}}}`

func defaultPolicy() Policy {
	return Policy{PartyPrefix: validatorPrefix, MaxRevoke: MaxRevoke}
}

func TestClassifyPreservesAdminAndValidatorActAs(t *testing.T) {
	t.Parallel()
	held := decodeRights(t,
		adminRight,
		actAs("a-validator-1::1220abcd"),
		actAs("alice-4f2a9c31::1220abcd"),
	)

	plan := Classify(held, defaultPolicy())

	if len(plan.Keep) != 2 {
		t.Fatalf("kept %d rights, want 2: %+v", len(plan.Keep), plan.Keep)
	}
	if plan.Keep[0].Kind() != AdminKind {
		t.Errorf("first kept right kind = %q, want %q", plan.Keep[0].Kind(), AdminKind)
	}
	if plan.Keep[1].Party() != "a-validator-1::1220abcd" {
		t.Errorf("second kept right party = %q", plan.Keep[1].Party())
	}
	if len(plan.Revoke) != 1 || plan.Revoke[0].Party() != "alice-4f2a9c31::1220abcd" {
		t.Errorf("revoke list = %+v, want the one ephemeral party", plan.Revoke)
	}
}

func TestClassifyRevokesNonActAsKindsOnTheValidatorParty(t *testing.T) {
	t.Parallel()
	held := decodeRights(t,
		adminRight,
		readAs("a-validator-1::1220abcd"),
		`{"kind":{"CanReadAsAnyParty":{"value":{}}}}`,
	)

	plan := Classify(held, defaultPolicy())

	if len(plan.Keep) != 1 {
		t.Errorf("kept %d rights, want only the admin right", len(plan.Keep))
	}
	if len(plan.Revoke) != 2 {
		t.Errorf("revoke list = %+v, want both non-act-as rights", plan.Revoke)
	}
}

func TestClassifyWithoutPartyPrefixPreservesNothingButAdmin(t *testing.T) {
	t.Parallel()
	held := decodeRights(t, adminRight, actAs("a-validator-1::1220abcd"))

	plan := Classify(held, Policy{MaxRevoke: MaxRevoke})

	if len(plan.Keep) != 1 || plan.Keep[0].Kind() != AdminKind {
		t.Errorf("kept %+v, want only the admin right", plan.Keep)
	}
	if len(plan.Revoke) != 1 {
		t.Errorf("revoke list = %+v, want the act-as right", plan.Revoke)
	}
}

func TestValidateRefusesWithoutAdminRight(t *testing.T) {
	t.Parallel()
	held := decodeRights(t, actAs("alice-4f2a9c31::1220abcd"))

	err := Classify(held, defaultPolicy()).Validate(defaultPolicy())

	if !errors.Is(err, ErrNoAdminRight) {
		t.Fatalf("validate error = %v, want ErrNoAdminRight", err)
	}
}

func TestValidateRefusesAboveTheBound(t *testing.T) {
	t.Parallel()
	entries := []string{adminRight, actAs(validatorPrefix + "1220abcd")}
	for i := 0; i < 4; i++ {
		entries = append(entries, actAs("alice-4f2a9c3"+string(rune('0'+i))+"::1220abcd"))
	}
	policy := Policy{PartyPrefix: validatorPrefix, MaxRevoke: 3}

	err := Classify(decodeRights(t, entries...), policy).Validate(policy)

	if err == nil || !strings.Contains(err.Error(), "exceeds the 3 bound") {
		t.Fatalf("validate error = %v, want the revoke bound", err)
	}
}

func TestValidateAcceptsAPlanAtTheBound(t *testing.T) {
	t.Parallel()
	policy := Policy{PartyPrefix: validatorPrefix, MaxRevoke: 1}
	held := decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice-4f2a9c31::1220abcd"))

	if err := Classify(held, policy).Validate(policy); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateRefusesWhenNoActAsRightOnThePreservedPartySurvives(t *testing.T) {
	t.Parallel()
	held := decodeRights(t, adminRight, actAs("sv::1220abcd"), actAs("alice-4f2a9c31::1220abcd"))
	policy := Policy{PartyPrefix: "sv-validator-1::", MaxRevoke: MaxRevoke}

	err := Classify(held, policy).Validate(policy)

	if !errors.Is(err, ErrNoOwnPartyRight) {
		t.Fatalf("validate error = %v, want ErrNoOwnPartyRight", err)
	}
	if !strings.Contains(err.Error(), "sv-validator-1::") {
		t.Errorf("error should name the preserved party, got %v", err)
	}
}

func TestValidateAdminRightAloneIsNotEnough(t *testing.T) {
	t.Parallel()
	held := decodeRights(t, adminRight, readAs("a-validator-1::1220abcd"))

	err := Classify(held, defaultPolicy()).Validate(defaultPolicy())

	if !errors.Is(err, ErrNoOwnPartyRight) {
		t.Fatalf("validate error = %v — a non-empty Keep must not satisfy the rail", err)
	}
}

func TestNewSinceReportsRightsTheConsentedPlanDidNotList(t *testing.T) {
	t.Parallel()
	consented := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice::1220abcd")), defaultPolicy())
	live := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice::1220abcd"), actAs("bob::1220abcd")), defaultPolicy())

	added := live.NewSince(consented)

	if len(added) != 1 || added[0].Party() != "bob::1220abcd" {
		t.Fatalf("NewSince = %+v, want the one right that appeared", added)
	}
}

func TestNewSinceComparesMembershipNotCounts(t *testing.T) {
	t.Parallel()
	consented := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice::1220abcd")), defaultPolicy())
	live := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("bob::1220abcd")), defaultPolicy())

	added := live.NewSince(consented)

	if len(added) != 1 || added[0].Party() != "bob::1220abcd" {
		t.Fatalf("NewSince = %+v — equal counts with different members must not pass", added)
	}
}

func TestNewSinceAcceptsAShrunkPlan(t *testing.T) {
	t.Parallel()
	consented := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice::1220abcd"), actAs("bob::1220abcd")), defaultPolicy())
	live := Classify(decodeRights(t, adminRight, actAs(validatorPrefix+"1220abcd"), actAs("alice::1220abcd")), defaultPolicy())

	if added := live.NewSince(consented); len(added) != 0 {
		t.Fatalf("NewSince = %+v, want nothing new in a subset", added)
	}
}

func TestRevokeGroupsCollapseEphemeralPartyFamilies(t *testing.T) {
	t.Parallel()
	held := decodeRights(t,
		adminRight,
		actAs("grpc-writer-parity-4f2a9c31::1220abcd"),
		actAs("grpc-writer-parity-9b1e07d4::1220abcd"),
		actAs("grpc-writer-parity-0011223344::1220abcd"),
		actAs("alice::1220abcd"),
		readAs("bob-aabbccdd::1220abcd"),
	)

	groups := Classify(held, defaultPolicy()).RevokeGroups()

	want := []Group{
		{Kind: "CanActAs", Family: "grpc-writer-parity-*", Count: 3},
		{Kind: "CanActAs", Family: "alice", Count: 1},
		{Kind: "CanReadAs", Family: "bob-*", Count: 1},
	}
	if len(groups) != len(want) {
		t.Fatalf("groups = %+v, want %+v", groups, want)
	}
	for i := range want {
		if groups[i] != want[i] {
			t.Errorf("group %d = %+v, want %+v", i, groups[i], want[i])
		}
	}
}

func TestFamilyKeepsNonEphemeralHints(t *testing.T) {
	t.Parallel()
	for party, want := range map[string]string{
		"alice::1220abcd":                     "alice",
		"a-validator-1::1220abcd":             "a-validator-1",
		"party-1::1220abcd":                   "party-1",
		"grpc-writer-parity-4f2a9c31::1220ab": "grpc-writer-parity-*",
		"bare-party":                          "bare-party",
		"":                                    "",
	} {
		if got := Family(party); got != want {
			t.Errorf("Family(%q) = %q, want %q", party, got, want)
		}
	}
}

func TestRightRoundTripsTheParticipantDocument(t *testing.T) {
	t.Parallel()
	source := `{"kind":{"CanActAs":{"value":{"party":"alice::1220abcd","extra":"kept"}}}}`

	held := decodeRights(t, source)
	encoded, err := json.Marshal(held[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(encoded) != source {
		t.Errorf("round trip = %s, want %s", encoded, source)
	}
}

func TestRightRejectsAnUnreadableActAsParty(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"renamed value wrapper":  `{"kind":{"CanActAs":{"target":{"party":"alice::1220abcd"}}}}`,
		"party is not a string":  `{"kind":{"CanActAs":{"value":{"party":42}}}}`,
		"value is not an object": `{"kind":{"CanActAs":{"value":"alice::1220abcd"}}}`,
		"no party at all":        `{"kind":{"CanActAs":{"value":{}}}}`,
	} {
		var right Right
		err := json.Unmarshal([]byte(body), &right)
		if err == nil || !strings.Contains(err.Error(), "could not read the party") {
			t.Errorf("%s: unmarshal error = %v, want the unreadable-party refusal", name, err)
		}
	}
}

func TestRightAcceptsPartylessKinds(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		adminRight,
		`{"kind":{"ParticipantAdmin":{}}}`,
		`{"kind":{"CanReadAsAnyParty":{"value":{}}}}`,
		`{"kind":{"IdentityProviderAdmin":{"value":{}}}}`,
		`{"kind":{"Empty":{}}}`,
	} {
		var right Right
		if err := json.Unmarshal([]byte(body), &right); err != nil {
			t.Errorf("%s: unmarshal: %v — kinds that carry no party must decode", body, err)
		}
		if right.Party() != "" {
			t.Errorf("%s: party = %q, want empty", body, right.Party())
		}
	}
	var admin Right
	if err := json.Unmarshal([]byte(adminRight), &admin); err != nil {
		t.Fatalf("unmarshal admin right: %v", err)
	}
	if admin.Kind() != AdminKind {
		t.Errorf("admin kind = %q, want %q", admin.Kind(), AdminKind)
	}
}

func TestRightRejectsAmbiguousKind(t *testing.T) {
	t.Parallel()
	var right Right

	err := json.Unmarshal([]byte(`{"kind":{"CanActAs":{},"CanReadAs":{}}}`), &right)

	if err == nil || !strings.Contains(err.Error(), "exactly one kind") {
		t.Fatalf("unmarshal error = %v, want the one-kind guard", err)
	}
}

func TestDefaultBoundClearsASaturatedParticipant(t *testing.T) {
	t.Parallel()
	entries := []string{adminRight, actAs(validatorPrefix + "1220abcd")}
	for i := 0; i < 999; i++ {
		entries = append(entries, actAs(fmt.Sprintf("leaked-%08x::1220abcd", i)))
	}
	policy := defaultPolicy()

	plan := Classify(decodeRights(t, entries...), policy)

	if len(plan.Revoke) != 999 {
		t.Fatalf("revoke list = %d rights, want 999", len(plan.Revoke))
	}
	if err := plan.Validate(policy); err != nil {
		t.Fatalf("validate: %v — the default bound must not block the saturated participant this tool exists to recover", err)
	}
}
