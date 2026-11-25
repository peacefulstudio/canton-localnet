// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"strings"
	"testing"
	"time"
)

var vettingCheckInstant = time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)

func windowedEntry(packageID string, bounds string) string {
	return `{"packages":[{"packageId":"` + packageID + `"` + bounds + `}],"participantId":"p1::122a","synchronizerId":"global::122a"}`
}

func vettedAt(t *testing.T, instant time.Time, entry string) (bool, error) {
	t.Helper()
	stub := newVettingStub(t, `{"vettedPackages":[`+entry+`]}`)
	client, err := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"}, withAdminClock(func() time.Time { return instant }))
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	return client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
}

func TestJsonLedgerAdminClient_IsPackageVetted_HonoursTheValidityWindow(t *testing.T) {
	cases := []struct {
		name   string
		bounds string
		want   bool
	}{
		{"no bounds", ``, true},
		{"null bounds", `,"validFromInclusive":null,"validUntilExclusive":null`, true},
		{"empty bounds", `,"validFromInclusive":"","validUntilExclusive":""`, true},
		{"inside the window", `,"validFromInclusive":"2026-11-24T11:00:00Z","validUntilExclusive":"2026-11-24T13:00:00Z"`, true},
		{"starting exactly now", `,"validFromInclusive":"2026-11-24T12:00:00Z"`, true},
		{"starting in the future", `,"validFromInclusive":"2026-11-24T12:00:00.000001Z"`, false},
		{"ending exactly now", `,"validUntilExclusive":"2026-11-24T12:00:00Z"`, false},
		{"ended in the past", `,"validUntilExclusive":"2026-11-24T11:59:59Z"`, false},
		{"open ended and started", `,"validFromInclusive":"2026-01-01T00:00:00Z"`, true},
		{"offset timestamp inside the window", `,"validUntilExclusive":"2026-11-24T13:00:01+01:00"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vetted, err := vettedAt(t, vettingCheckInstant, windowedEntry("pkg-1", tc.bounds))
			if err != nil {
				t.Fatalf("isPackageVetted: %v", err)
			}
			if vetted != tc.want {
				t.Errorf("vetted = %v, want %v", vetted, tc.want)
			}
		})
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_FindsALaterEntryInsideTheWindowAfterAnExpiredOne(t *testing.T) {
	expired := windowedEntry("pkg-1", `,"validUntilExclusive":"2026-01-01T00:00:00Z"`)
	current := windowedEntry("pkg-1", `,"validFromInclusive":"2026-06-01T00:00:00Z"`)

	vetted, err := vettedAt(t, vettingCheckInstant, expired+","+current)

	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if !vetted {
		t.Error("vetted = false, want true from the entry inside its window")
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_RejectsAnUnparseableBoundOnAMatchingEntry(t *testing.T) {
	cases := []struct {
		name      string
		bounds    string
		wantField string
	}{
		{"from", `,"validFromInclusive":"yesterday"`, "validFromInclusive"},
		{"until", `,"validUntilExclusive":"2026-13-45"`, "validUntilExclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := vettedAt(t, vettingCheckInstant, windowedEntry("pkg-1", tc.bounds))

			if err == nil || !strings.Contains(err.Error(), tc.wantField) {
				t.Fatalf("error = %v, want one naming %s", err, tc.wantField)
			}
		})
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_IgnoresAnUnparseableBoundOnAnotherPackage(t *testing.T) {
	other := windowedEntry("pkg-2", `,"validFromInclusive":"yesterday"`)

	vetted, err := vettedAt(t, vettingCheckInstant, other)

	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if vetted {
		t.Error("vetted = true for another package, want false")
	}
}
