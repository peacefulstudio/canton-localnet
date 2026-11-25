// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestValidatorFixture_CreateUserAfterTeardownFailsAndLeavesTheParticipantUntouched(t *testing.T) {
	a := newScriptedLedger(t, nil)
	b := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, a, b)
	views := map[Role]*ValidatorFixture{
		RoleAValidator1: f.MustValidator(RoleAValidator1),
		RoleBValidator1: f.MustValidator(RoleBValidator1),
	}
	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	for role, view := range views {
		_, err := view.CreateUser(context.Background(), UserOptions{UserID: "late-user", PrimaryParty: "late::ns"})

		if err == nil || !strings.Contains(err.Error(), "torn down") {
			t.Errorf("%s CreateUser after Teardown error = %v, want one saying the view was torn down", role, err)
		}
	}
	for name, ledger := range map[string]*scriptedLedger{"a": a, "b": b} {
		if hits := ledger.recorded(); len(hits) != 0 {
			t.Errorf("ledger %s received %v after Teardown, want no request", name, hits)
		}
	}
}

func TestValidatorFixture_CreateUserStillWorksWhenTeardownFailed(t *testing.T) {
	ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{http.StatusServiceUnavailable, "{}"} })
	f := newTeardownFixture(t, ledger, nil)
	view := f.MustValidator(RoleAValidator1)
	createUser(t, view.CreateUser, "alice-user")
	if err := f.Teardown(context.Background()); err == nil {
		t.Fatal("Teardown = nil, want the 503")
	}

	createUser(t, view.CreateUser, "bob-user")
}

func TestFixture_ViewFromASecondSetupCanCreateUsersAfterAnEarlierTeardown(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)
	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if err := f.Setup(context.Background()); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	createUser(t, f.MustValidator(RoleAValidator1).CreateUser, "fresh-user")
	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("second Teardown: %v", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"fresh-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}
