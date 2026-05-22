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

func TestFetchParticipantIDSendsBearer(t *testing.T) {
	t.Parallel()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"participantId":"a-validator-1::abcd"}`))
	}))
	defer srv.Close()

	ep := Endpoints{JSONLedgerAPIURL: srv.URL}
	id, err := FetchParticipantID(context.Background(), ep, "tok", srv.Client())
	if err != nil {
		t.Fatalf("FetchParticipantID: %v", err)
	}
	if id != "a-validator-1::abcd" {
		t.Errorf("id: got %q", id)
	}
	if got != "Bearer tok" {
		t.Errorf("auth header: got %q", got)
	}
}

func TestFetchParticipantIDSurfaces401(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()
	_, err := FetchParticipantID(context.Background(), Endpoints{JSONLedgerAPIURL: srv.URL}, "tok", srv.Client())
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %q", err.Error())
	}
}

func TestSplitParticipantID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    string
		wantHint string
		wantNs   string
		wantErr  bool
	}{
		{name: "valid", input: "a-validator-1::abcd1234", wantHint: "a-validator-1", wantNs: "abcd1234"},
		{name: "missing separator", input: "nope", wantErr: true},
		{name: "empty hint", input: "::abcd1234", wantErr: true},
		{name: "empty namespace", input: "a-validator-1::", wantErr: true},
		{name: "empty both", input: "::", wantErr: true},
		{name: "triple colon takes first ::", input: "a:::ns", wantHint: "a", wantNs: ":ns"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hint, ns, err := SplitParticipantID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if hint != tc.wantHint || ns != tc.wantNs {
				t.Errorf("got (%q,%q), want (%q,%q)", hint, ns, tc.wantHint, tc.wantNs)
			}
		})
	}
}
