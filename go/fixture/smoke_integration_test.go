// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fixture

import (
	"context"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSmoke_GetParticipantId(t *testing.T) {
	skipIfStackUnreachable(t, RoleAValidator1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	f, err := New(Config{Role: RoleAValidator1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() {
		_ = f.Teardown(context.Background())
	})

	id, err := f.GetParticipantId(ctx)
	if err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		t.Fatal("participantId is empty")
	}
	t.Logf("participantId = %s", id)
}

func TestSmoke_AllocatePartyUploadDarBuildUser(t *testing.T) {
	skipIfStackUnreachable(t, RoleAValidator1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	f, err := New(Config{Role: RoleAValidator1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() {
		_ = f.Teardown(context.Background())
	})

	suffix := f.PartyAllocator().Suffix()
	if suffix == "" {
		t.Fatal("PartyAllocator suffix is empty")
	}
	t.Logf("party suffix = %s", suffix)

	party, err := f.AllocateParty(ctx, "cln-go-it", "Smoke Test Party")
	if err != nil {
		t.Fatalf("AllocateParty: %v", err)
	}
	if strings.TrimSpace(party) == "" {
		t.Fatal("allocated party id is empty")
	}
	if !strings.Contains(party, "cln-go-it-"+suffix) {
		t.Errorf("party id %q does not contain prefix-suffix hint cln-go-it-%s", party, suffix)
	}
	t.Logf("allocated party = %s", party)

	if darPath := os.Getenv("CANTON_LOCALNET_TEST_DAR_PATH"); darPath != "" {
		t.Logf("uploading DAR %s", darPath)
		if err := f.UploadDar(ctx, darPath); err != nil {
			t.Fatalf("UploadDar: %v", err)
		}
		t.Logf("uploading the same DAR again to exercise KNOWN_PACKAGE_VERSION idempotency")
		if err := f.UploadDar(ctx, darPath); err != nil {
			t.Fatalf("UploadDar (second pass, idempotency): %v", err)
		}
	} else {
		t.Log("CANTON_LOCALNET_TEST_DAR_PATH not set — skipping DAR upload portion")
	}

	userID := "smoke-user-" + suffix
	id, err := f.CreateUser(ctx, UserOptions{
		UserID:       userID,
		PrimaryParty: party,
		ActAs:        []string{party},
		ReadAs:       []string{party},
		Annotations:  map[string]string{"username": userID},
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id != userID {
		t.Errorf("returned id = %q, want %q", id, userID)
	}
	t.Logf("created user = %s with actAs/readAs on %s", id, party)
}

func skipIfStackUnreachable(t *testing.T, role Role) {
	t.Helper()
	endpoints, err := NewEndpointDiscovery().For(role)
	if err != nil {
		t.Skipf("skipping smoke: endpoint discovery failed: %v", err)
	}
	u, err := url.Parse(endpoints.JSONLedgerAPIURL)
	if err != nil {
		t.Skipf("skipping smoke: cannot parse JSONLedgerAPIURL %q: %v", endpoints.JSONLedgerAPIURL, err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
	if err != nil {
		t.Skipf("skipping smoke: %s unreachable (%v) — bring up compose stack with `make up && make wait-ready`", u.Host, err)
	}
	_ = conn.Close()
}
