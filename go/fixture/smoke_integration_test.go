// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fixture

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSmoke_GetParticipantId(t *testing.T) {
	skipIfStackUnreachable(t, RoleAppProvider)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	f, err := New(Config{Role: RoleAppProvider})
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
