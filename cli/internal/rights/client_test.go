// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package rights

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListReadsTheUsersRights(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"rights":[` + adminRight + `,` + actAs("alice::1220abcd") + `]}`))
	}))
	defer server.Close()

	held, err := Client{BaseURL: server.URL, Token: "tok"}.List(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotPath != "/v2/users/user-1/rights" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if len(held) != 2 {
		t.Fatalf("held %d rights, want 2", len(held))
	}
}

func TestListReportsAnHTTPFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"cause":"NOT_FOUND"}`))
	}))
	defer server.Close()

	_, err := Client{BaseURL: server.URL, Token: "tok"}.List(context.Background(), "ghost")

	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("list error = %v, want the 404 surfaced", err)
	}
	if strings.Contains(err.Error(), "tok") {
		t.Errorf("error leaked the bearer token: %v", err)
	}
}

func TestRevokePatchesTheRequestedRights(t *testing.T) {
	t.Parallel()
	var gotMethod, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		var request struct {
			Rights []json.RawMessage `json:"rights"`
		}
		_ = json.Unmarshal(body, &request)
		echoed, _ := json.Marshal(map[string]any{"newlyRevokedRights": request.Rights})
		_, _ = w.Write(echoed)
	}))
	defer server.Close()

	revoke := decodeRights(t, actAs("alice::1220abcd"), actAs("bob::1220abcd"))
	revoked, err := Client{BaseURL: server.URL, Token: "tok"}.Revoke(context.Background(), "user-1", revoke)

	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if !strings.Contains(gotBody, `"userId":"user-1"`) {
		t.Errorf("request body = %s, want the user id", gotBody)
	}
	if !strings.Contains(gotBody, `"party":"alice::1220abcd"`) {
		t.Errorf("request body = %s, want the participant's own right documents", gotBody)
	}
	if len(revoked) != 2 {
		t.Errorf("revoked %d rights, want 2", len(revoked))
	}
}

func TestRevokeFailsOnAPartialRevoke(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"newlyRevokedRights":[` + actAs("alice::1220abcd") + `]}`))
	}))
	defer server.Close()

	revoke := decodeRights(t, actAs("alice::1220abcd"), actAs("bob::1220abcd"))
	_, err := Client{BaseURL: server.URL, Token: "tok"}.Revoke(context.Background(), "user-1", revoke)

	if err == nil || !strings.Contains(err.Error(), "revoked 1 of 2") {
		t.Fatalf("revoke error = %v, want the partial-revoke failure", err)
	}
}

func TestRevokeFailsWhenTheParticipantConfirmsTheWrongRight(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"newlyRevokedRights":[` + actAs("alice::1220abcd") + `,` + actAs("carol::1220abcd") + `]}`))
	}))
	defer server.Close()

	revoke := decodeRights(t, actAs("alice::1220abcd"), actAs("bob::1220abcd"))
	_, err := Client{BaseURL: server.URL, Token: "tok"}.Revoke(context.Background(), "user-1", revoke)

	if err == nil || !strings.Contains(err.Error(), "revoked 1 of 2") {
		t.Fatalf("revoke error = %v, want the partial-revoke failure even though the response has the requested count", err)
	}
}

func TestRevokeEscapesTheUserIDInThePath(t *testing.T) {
	t.Parallel()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"rights":[]}`))
	}))
	defer server.Close()

	if _, err := (Client{BaseURL: server.URL, Token: "tok"}).List(context.Background(), "a/b c"); err != nil {
		t.Fatalf("list: %v", err)
	}

	if gotPath != "/v2/users/a%2Fb%20c/rights" {
		t.Errorf("path = %q, want the user id escaped", gotPath)
	}
}

func TestPrimaryPartyReadsTheUsersOwnParty(t *testing.T) {
	t.Parallel()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"user":{"id":"user-1","primaryParty":"sv::1220abcd"}}`))
	}))
	defer server.Close()

	party, err := Client{BaseURL: server.URL, Token: "tok"}.PrimaryParty(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("primary party: %v", err)
	}
	if gotPath != "/v2/users/user-1" {
		t.Errorf("path = %q", gotPath)
	}
	if party != "sv::1220abcd" {
		t.Errorf("primary party = %q, want the party the participant reports", party)
	}
}

func TestPrimaryPartyIsEmptyWhenTheUserHasNone(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"user-1"}}`))
	}))
	defer server.Close()

	party, err := Client{BaseURL: server.URL, Token: "tok"}.PrimaryParty(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("primary party: %v", err)
	}
	if party != "" {
		t.Errorf("primary party = %q, want empty", party)
	}
}
