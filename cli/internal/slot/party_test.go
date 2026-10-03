// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPrimaryPartyReadsUserPrimaryParty(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"user":{"id":"u-1","primaryParty":"a-validator-1::abcd"}}`))
	}))
	defer srv.Close()

	ep := Endpoints{JSONLedgerAPIURL: srv.URL, ValidatorUserID: "u-1"}
	party, err := FetchPrimaryParty(context.Background(), ep, "tok", srv.Client())
	if err != nil {
		t.Fatalf("FetchPrimaryParty: %v", err)
	}
	if party != "a-validator-1::abcd" {
		t.Errorf("party: got %q", party)
	}
	if gotPath != "/v2/users/u-1" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header: got %q", gotAuth)
	}
}

func TestFetchPrimaryPartyFailsWhenUserHasNoPrimaryParty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"id":"u-1"}}`))
	}))
	defer srv.Close()
	_, err := FetchPrimaryParty(context.Background(), Endpoints{JSONLedgerAPIURL: srv.URL, ValidatorUserID: "u-1"}, "tok", srv.Client())
	if err == nil || !strings.Contains(err.Error(), "no primaryParty") {
		t.Fatalf("expected a no-primaryParty error, got %v", err)
	}
}

func TestFetchPrimaryPartySurfacesStatusAndEndpointNotBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("user-body-marker"))
	}))
	defer srv.Close()
	_, err := FetchPrimaryParty(context.Background(), Endpoints{JSONLedgerAPIURL: srv.URL, ValidatorUserID: "u-1"}, "tok", srv.Client())
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("expected status + endpoint in error, got %v", err)
	}
	if strings.Contains(err.Error(), "user-body-marker") {
		t.Errorf("error must not echo the response body, got %q", err.Error())
	}
}

func TestFetchPrimaryPartyRequiresValidatorUserID(t *testing.T) {
	t.Parallel()
	_, err := FetchPrimaryParty(context.Background(), Endpoints{}, "tok", nil)
	if err == nil {
		t.Fatal("expected an error when the validator user id is empty")
	}
}

func TestWebUIPortFollowsSlotPrefix(t *testing.T) {
	t.Parallel()
	want := map[string]string{"sv": "10000", "a": "11000", "b": "12000", "c": "13000", "d": "14000"}
	for short, port := range want {
		s, err := Parse(short)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.WebUIPort(); got != port {
			t.Errorf("%s WebUIPort: got %q, want %q", short, got, port)
		}
	}
}
