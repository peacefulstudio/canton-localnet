// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/slot"
)

func TestFetchConnectedSynchronizers_ParsesAliasesAndIds(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[`+
			`{"synchronizerAlias":"global","synchronizerId":"global::122a"},`+
			`{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}`)
	}))
	t.Cleanup(server.Close)

	ep := slot.Endpoints{JSONLedgerAPIURL: server.URL}
	syncs, err := slot.FetchConnectedSynchronizers(context.Background(), ep, "tok", server.Client())
	if err != nil {
		t.Fatalf("FetchConnectedSynchronizers: %v", err)
	}
	if len(syncs) != 2 {
		t.Fatalf("len = %d, want 2", len(syncs))
	}
	if syncs[1].Alias != "app-synchronizer" || syncs[1].ID != "app-synchronizer::122b" {
		t.Fatalf("second = %+v, want app-synchronizer::122b", syncs[1])
	}
	if gotPath != "/v2/state/connected-synchronizers" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth = %q, want Bearer tok", gotAuth)
	}
}
