// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const (
	userNotFoundBody  = `{"code":"USER_NOT_FOUND","cause":"deleting user failed for unknown user \"gone\"","errorCategory":11}`
	otherNotFoundBody = `{"code":"PACKAGE_NOT_FOUND","cause":"not a user"}`
)

type ledgerReply struct {
	status int
	body   string
}

type ledgerHit struct {
	method string
	path   string
}

type scriptedLedger struct {
	server      *httptest.Server
	onDelete    func(userID string, call int) ledgerReply
	mu          sync.Mutex
	hits        []ledgerHit
	deleteCalls map[string]int
}

func newScriptedLedger(t *testing.T, onDelete func(userID string, call int) ledgerReply) *scriptedLedger {
	t.Helper()
	if onDelete == nil {
		onDelete = func(string, int) ledgerReply { return ledgerReply{status: http.StatusOK, body: "{}"} }
	}
	l := &scriptedLedger{onDelete: onDelete, deleteCalls: map[string]int{}}
	l.server = httptest.NewServer(http.HandlerFunc(l.serve))
	t.Cleanup(l.server.Close)
	return l
}

func (l *scriptedLedger) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	l.mu.Lock()
	l.hits = append(l.hits, ledgerHit{method: r.Method, path: r.URL.Path})
	call := 0
	if r.Method == http.MethodDelete {
		l.deleteCalls[r.URL.Path]++
		call = l.deleteCalls[r.URL.Path]
	}
	l.mu.Unlock()

	reply := ledgerReply{status: http.StatusOK, body: "{}"}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
		var req createUserRequest
		_ = json.Unmarshal(body, &req)
		reply.body = `{"user":{"id":"` + req.User.ID + `"}}`
	case r.Method == http.MethodDelete:
		reply = l.onDelete(strings.TrimPrefix(r.URL.Path, "/v2/users/"), call)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(reply.status)
	_, _ = io.WriteString(w, reply.body)
}

func (l *scriptedLedger) recorded() []ledgerHit {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]ledgerHit(nil), l.hits...)
}

func (l *scriptedLedger) deletedUserIDs() []string {
	var ids []string
	for _, hit := range l.recorded() {
		if hit.method == http.MethodDelete {
			ids = append(ids, strings.TrimPrefix(hit.path, "/v2/users/"))
		}
	}
	return ids
}

func (l *scriptedLedger) port(t *testing.T) string {
	t.Helper()
	_, port := splitHostPort(t, l.server.URL)
	return port
}

func newTeardownFixture(t *testing.T, a, b *scriptedLedger) *Fixture {
	t.Helper()
	keycloak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-stub", "token_type": "Bearer", "expires_in": 300})
	}))
	t.Cleanup(keycloak.Close)
	host, _ := splitHostPort(t, a.server.URL)
	keycloakHost, keycloakPort := splitHostPort(t, keycloak.URL)
	env := map[string]string{
		"CANTON_LOCALNET_HOST":                        host,
		"CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT":     a.port(t),
		"CANTON_LOCALNET_KEYCLOAK_HOST":               keycloakHost,
		"CANTON_LOCALNET_KEYCLOAK_PORT":               keycloakPort,
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID":     "cid-a",
		"CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET": "csecret-a",
	}
	if b != nil {
		env["CANTON_LOCALNET_B_VALIDATOR_1_JSON_PORT"] = b.port(t)
		env["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID"] = "cid-b"
		env["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET"] = "csecret-b"
	}
	return newSetupFixture(t, RoleAValidator1, env)
}

func createUser(t *testing.T, create func(context.Context, UserOptions) (string, error), id string) {
	t.Helper()
	if _, err := create(context.Background(), UserOptions{UserID: id, PrimaryParty: id + "::ns"}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

func assertStatusError(t *testing.T, err error, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want HTTP %d", wantStatus)
	}
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.statusCode != wantStatus {
		t.Fatalf("error = %v, want HTTP %d", err, wantStatus)
	}
}

func TestUserBuilder_DeleteSendsOneDeleteToTheEscapedUserPath(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []*http.Request
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r)
		mu.Unlock()
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(server.Close)
	b, err := NewUserBuilder(server.URL, staticTokenProvider{token: "tok-user"})
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}

	if err := b.deleteUser(context.Background(), "alice@corp/1", 1); err != nil {
		t.Fatalf("deleteUser: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	got := requests[0]
	if got.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.Method)
	}
	if got.URL.EscapedPath() != "/v2/users/alice@corp%2F1" {
		t.Errorf("escaped path = %q, want /v2/users/alice@corp%%2F1", got.URL.EscapedPath())
	}
	if got.Header.Get("Authorization") != "Bearer tok-user" {
		t.Errorf("Authorization = %q, want Bearer tok-user", got.Header.Get("Authorization"))
	}
}

func TestUserBuilder_DeleteFirstAttemptFailsOnAnyNonSuccess(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"missing user", http.StatusNotFound, userNotFoundBody},
		{"other 404", http.StatusNotFound, otherNotFoundBody},
		{"server error", http.StatusInternalServerError, "{}"},
		{"unavailable without body", http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{tc.status, tc.body} })
			b, _ := NewUserBuilder(ledger.server.URL, staticTokenProvider{token: "tok"})

			err := b.deleteUser(context.Background(), "alice-user", 1)

			assertStatusError(t, err, tc.status)
			if !strings.Contains(err.Error(), "/v2/users/alice-user") {
				t.Errorf("error %q does not name the user path", err)
			}
		})
	}
}

func TestUserBuilder_DeleteRetryTreatsMissingUserAsSuccess(t *testing.T) {
	ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{http.StatusNotFound, userNotFoundBody} })
	b, _ := NewUserBuilder(ledger.server.URL, staticTokenProvider{token: "tok"})

	if err := b.deleteUser(context.Background(), "alice-user", 2); err != nil {
		t.Fatalf("deleteUser(attempt 2) = %v, want nil for a missing user", err)
	}
}

func TestUserBuilder_DeleteRetryStillFailsWhenTheResponseIsNotAMissingUser(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"other 404", http.StatusNotFound, otherNotFoundBody},
		{"404 without body", http.StatusNotFound, ""},
		{"500 carrying the missing-user code", http.StatusInternalServerError, userNotFoundBody},
		{"unavailable", http.StatusServiceUnavailable, "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{tc.status, tc.body} })
			b, _ := NewUserBuilder(ledger.server.URL, staticTokenProvider{token: "tok"})

			assertStatusError(t, b.deleteUser(context.Background(), "alice-user", 2), tc.status)
		})
	}
}

func TestFixture_TeardownDeletesEachUserCreatedThroughCreateUser(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")
	createUser(t, f.CreateUser, "bob-user")

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user", "bob-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownIssuesNoRequestWhenNoUserWasCreated(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if hits := ledger.recorded(); len(hits) != 0 {
		t.Errorf("requests = %v, want none", hits)
	}
}

func TestFixture_TeardownLeavesUsersCreatedThroughUserBuilderDirectly(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.UserBuilder().Create, "direct-user")

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if ids := ledger.deletedUserIDs(); len(ids) != 0 {
		t.Errorf("deleted = %v, want none", ids)
	}
}

func TestFixture_TeardownIssuesNoGetRequest(t *testing.T) {
	ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{http.StatusServiceUnavailable, "{}"} })
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")

	_ = f.Teardown(context.Background())
	_ = f.Teardown(context.Background())

	for _, hit := range ledger.recorded() {
		if hit.method == http.MethodGet {
			t.Errorf("unexpected %s %s", hit.method, hit.path)
		}
	}
	if got := len(ledger.deletedUserIDs()); got != 2 {
		t.Errorf("DELETE count = %d, want 2", got)
	}
}

func TestFixture_TeardownFailsWhenTheFirstDeleteAttemptFails(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"missing user", http.StatusNotFound, userNotFoundBody},
		{"other 404", http.StatusNotFound, otherNotFoundBody},
		{"server error", http.StatusInternalServerError, "{}"},
		{"unavailable without body", http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newScriptedLedger(t, func(string, int) ledgerReply { return ledgerReply{tc.status, tc.body} })
			f := newTeardownFixture(t, ledger, nil)
			createUser(t, f.CreateUser, "alice-user")

			err := f.Teardown(context.Background())

			assertStatusError(t, err, tc.status)
			if !strings.Contains(err.Error(), "alice-user") {
				t.Errorf("error %q does not name the user", err)
			}
		})
	}
}

func TestFixture_TeardownCountsAttemptsPerUserAcrossTeardownCycles(t *testing.T) {
	ledger := newScriptedLedger(t, func(userID string, _ int) ledgerReply {
		if userID == "first-cycle-user" {
			return ledgerReply{http.StatusNoContent, ""}
		}
		return ledgerReply{http.StatusNotFound, userNotFoundBody}
	})
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "first-cycle-user")
	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("first Teardown = %v, want nil", err)
	}
	if err := f.Setup(context.Background()); err != nil {
		t.Fatalf("second Setup: %v", err)
	}
	createUser(t, f.CreateUser, "second-cycle-user")

	err := f.Teardown(context.Background())

	assertStatusError(t, err, http.StatusNotFound)
}

func TestFixture_TeardownRetryTreatsAMissingUserAsSuccess(t *testing.T) {
	ledger := newScriptedLedger(t, func(_ string, call int) ledgerReply {
		if call == 1 {
			return ledgerReply{http.StatusServiceUnavailable, "{}"}
		}
		return ledgerReply{http.StatusNotFound, userNotFoundBody}
	})
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")
	if err := f.Teardown(context.Background()); err == nil {
		t.Fatal("first Teardown = nil, want the 503")
	}

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("second Teardown = %v, want nil for a missing user", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user", "alice-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownRetryStillFailsWhenTheResponseIsNotAMissingUser(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"other 404", http.StatusNotFound, otherNotFoundBody},
		{"404 without body", http.StatusNotFound, ""},
		{"500 carrying the missing-user code", http.StatusInternalServerError, userNotFoundBody},
		{"unavailable", http.StatusServiceUnavailable, "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newScriptedLedger(t, func(_ string, call int) ledgerReply {
				if call == 1 {
					return ledgerReply{http.StatusServiceUnavailable, "{}"}
				}
				return ledgerReply{tc.status, tc.body}
			})
			f := newTeardownFixture(t, ledger, nil)
			createUser(t, f.CreateUser, "alice-user")
			_ = f.Teardown(context.Background())

			assertStatusError(t, f.Teardown(context.Background()), tc.status)
		})
	}
}

func TestFixture_TeardownRetryDeletesOnlyTheUsersStillOutstanding(t *testing.T) {
	ledger := newScriptedLedger(t, func(user string, call int) ledgerReply {
		if user == "bob-user" && call == 1 {
			return ledgerReply{http.StatusServiceUnavailable, "{}"}
		}
		return ledgerReply{http.StatusOK, "{}"}
	})
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")
	createUser(t, f.CreateUser, "bob-user")
	_ = f.Teardown(context.Background())

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("second Teardown: %v", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user", "bob-user", "bob-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownDeletesTheRemainingUsersWhenOneDeleteFails(t *testing.T) {
	ledger := newScriptedLedger(t, func(user string, _ int) ledgerReply {
		if user == "alice-user" {
			return ledgerReply{http.StatusServiceUnavailable, "{}"}
		}
		return ledgerReply{http.StatusOK, "{}"}
	})
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")
	createUser(t, f.CreateUser, "bob-user")

	if err := f.Teardown(context.Background()); err == nil {
		t.Fatal("Teardown = nil, want the 503 for alice-user")
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user", "bob-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownIssuesNoFurtherRequestOnceEveryUserIsDeleted(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.CreateUser, "alice-user")
	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("second Teardown: %v", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownDeletesUsersCreatedThroughTheDefaultRoleViewOnce(t *testing.T) {
	ledger := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, ledger, nil)
	createUser(t, f.MustValidator(RoleAValidator1).CreateUser, "alice-user")

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if got, want := ledger.deletedUserIDs(), []string{"alice-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownDeletesUsersCreatedThroughAnotherRoleViewOnThatRole(t *testing.T) {
	a := newScriptedLedger(t, nil)
	b := newScriptedLedger(t, nil)
	f := newTeardownFixture(t, a, b)
	createUser(t, f.CreateUser, "alice-user")
	createUser(t, f.MustValidator(RoleBValidator1).CreateUser, "bob-user")

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if got, want := a.deletedUserIDs(), []string{"alice-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("role a deleted = %v, want %v", got, want)
	}
	if got, want := b.deletedUserIDs(), []string{"bob-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("role b deleted = %v, want %v", got, want)
	}
}

func TestFixture_TeardownRetryReachesUsersOnAnotherRoleAfterAFailedFirstAttempt(t *testing.T) {
	a := newScriptedLedger(t, nil)
	b := newScriptedLedger(t, func(_ string, call int) ledgerReply {
		if call == 1 {
			return ledgerReply{http.StatusServiceUnavailable, "{}"}
		}
		return ledgerReply{http.StatusNotFound, userNotFoundBody}
	})
	f := newTeardownFixture(t, a, b)
	createUser(t, f.MustValidator(RoleBValidator1).CreateUser, "bob-user")
	if err := f.Teardown(context.Background()); err == nil {
		t.Fatal("first Teardown = nil, want the 503")
	}

	if err := f.Teardown(context.Background()); err != nil {
		t.Fatalf("second Teardown: %v", err)
	}

	if got, want := b.deletedUserIDs(), []string{"bob-user", "bob-user"}; !reflect.DeepEqual(got, want) {
		t.Errorf("role b deleted = %v, want %v", got, want)
	}
}
