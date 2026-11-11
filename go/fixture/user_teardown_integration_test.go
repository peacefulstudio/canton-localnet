// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fixture

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func newIntegrationFixture(t *testing.T, ctx context.Context) *Fixture {
	t.Helper()
	f, err := New(Config{Role: RoleAValidator1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return f
}

func TestTeardown_DeletesTheUsersItCreated(t *testing.T) {
	skipIfStackUnreachable(t, RoleAValidator1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	observer := newIntegrationFixture(t, ctx)
	t.Cleanup(func() { _ = observer.Teardown(context.Background()) })
	f := newIntegrationFixture(t, ctx)
	party, err := f.AllocateParty(ctx, "cln-go-teardown")
	if err != nil {
		t.Fatalf("AllocateParty: %v", err)
	}
	userID := "teardown-user-" + f.PartyAllocator().Suffix()
	if _, err := f.CreateUser(ctx, UserOptions{UserID: userID, PrimaryParty: party, ActAs: []string{party}}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if status := getUserStatus(t, ctx, observer, userID); status != http.StatusOK {
		t.Fatalf("GET user before Teardown = %d, want 200", status)
	}

	if err := f.Teardown(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if status := getUserStatus(t, ctx, observer, userID); status != http.StatusNotFound {
		t.Errorf("GET user after Teardown = %d, want 404", status)
	}
}

func TestDeleteUser_ReportsAMissingUserAs404UserNotFound(t *testing.T) {
	skipIfStackUnreachable(t, RoleAValidator1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f := newIntegrationFixture(t, ctx)
	t.Cleanup(func() { _ = f.Teardown(context.Background()) })

	err := f.UserBuilder().deleteUser(ctx, "never-created-"+f.PartyAllocator().Suffix(), 1)

	assertStatusError(t, err, http.StatusNotFound)
	if !isMissingUser(err) {
		t.Errorf("UserBuilder.deleteUser treats exactly '404 with code USER_NOT_FOUND' as a user that is already gone on retry; "+
			"the participant answered %v. If this fires after a Splice repin, that contract changed and UserBuilder must change with it", err)
	}
}

func getUserStatus(t *testing.T, ctx context.Context, f *Fixture, userID string) int {
	t.Helper()
	token, err := f.Tokens().Token(ctx)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.Endpoints().JSONLedgerAPIURL+"/v2/users/"+userID, nil)
	if err != nil {
		t.Fatalf("build GET: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET user: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
