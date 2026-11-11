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
	"strconv"
	"strings"
	"sync"
	"testing"
)

type staticTokenProvider struct {
	token string
	err   error
}

func (s staticTokenProvider) Token(ctx context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.token, nil
}

type countingTokenProvider struct {
	mu    sync.Mutex
	calls int
}

func (c *countingTokenProvider) Token(context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return "tok-" + strconv.Itoa(c.calls), nil
}

func TestJsonLedgerAdminClient_GetParticipantId_RequestShape(t *testing.T) {
	var (
		method string
		path   string
		auth   string
		accept string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::abc123"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok-xyz"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}

	id, err := client.GetParticipantId(context.Background())
	if err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if id != "participant::abc123" {
		t.Fatalf("id = %q, want participant::abc123", id)
	}
	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
	if path != "/v2/parties/participant-id" {
		t.Errorf("path = %q, want /v2/parties/participant-id", path)
	}
	if auth != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer tok-xyz")
	}
	if accept != "application/json" {
		t.Errorf("Accept = %q, want application/json", accept)
	}
}

func TestJsonLedgerAdminClient_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":"participant::abc"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL+"/", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err != nil {
		t.Fatalf("GetParticipantId: %v", err)
	}
	if path != "/v2/parties/participant-id" {
		t.Errorf("path = %q (got double slash?)", path)
	}
}

func TestJsonLedgerAdminClient_PropagatesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"forbidden"}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	_, err = client.GetParticipantId(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not include 403", err.Error())
	}
}

func TestJsonLedgerAdminClient_RejectsEmptyParticipantId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"participantId":""}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err == nil {
		t.Fatal("expected error for empty participantId")
	}
}

func TestJsonLedgerAdminClient_PropagatesTokenError(t *testing.T) {
	client, err := NewJsonLedgerAdminClient("http://localhost", staticTokenProvider{err: errors.New("boom")})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	_, err = client.GetParticipantId(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "acquire token") {
		t.Errorf("error %q does not wrap token-acquisition failure", err.Error())
	}
}

func TestJsonLedgerAdminClient_RejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `not json`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetParticipantId(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestJsonLedgerAdminClient_RequiresArguments(t *testing.T) {
	if _, err := NewJsonLedgerAdminClient("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewJsonLedgerAdminClient("http://x", nil); err == nil {
		t.Error("expected error for nil TokenProvider")
	}
}

func TestJsonLedgerAdminClient_GetConnectedSynchronizers(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[`+
			`{"synchronizerAlias":"global","synchronizerId":"global::122a"},`+
			`{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}

	id, err := client.GetAppSynchronizerId(context.Background(), "alice::122a")
	if err != nil {
		t.Fatalf("GetAppSynchronizerId: %v", err)
	}
	if id != "app-synchronizer::122b" {
		t.Fatalf("id = %q, want app-synchronizer::122b", id)
	}
	if gotPath != "/v2/state/connected-synchronizers?party=alice%3A%3A122a" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestJsonLedgerAdminClient_GetConnectedSynchronizers_RejectsEmptySynchronizerId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":""}]}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	if _, err := client.GetConnectedSynchronizers(context.Background(), "alice::122a"); err == nil {
		t.Fatal("expected error for entry missing synchronizerId")
	}
}

func TestJsonLedgerAdminClient_GetAppSynchronizerId_ErrorWhenAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}`)
	}))
	t.Cleanup(server.Close)

	client, _ := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	_, err := client.GetAppSynchronizerId(context.Background(), "alice::122a")
	if err == nil || !strings.Contains(err.Error(), "app-synchronizer") {
		t.Fatalf("err = %v, want mention of app-synchronizer", err)
	}
}

func TestJsonLedgerAdminClient_GetGlobalSynchronizerId_ListsParticipantWideAndPicksTheGlobalAlias(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[`+
			`{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"},`+
			`{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}

	id, err := client.GetGlobalSynchronizerId(context.Background())
	if err != nil {
		t.Fatalf("GetGlobalSynchronizerId: %v", err)
	}
	if id != "global::122a" {
		t.Fatalf("id = %q, want global::122a", id)
	}
	if gotPath != "/v2/state/connected-synchronizers?" {
		t.Fatalf("path = %q, want a participant-wide listing with no party filter", gotPath)
	}
}

func TestJsonLedgerAdminClient_GetGlobalSynchronizerId_ErrorWhenAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connectedSynchronizers":[{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}`)
	}))
	t.Cleanup(server.Close)

	client, _ := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})
	_, err := client.GetGlobalSynchronizerId(context.Background())
	if err == nil || !strings.Contains(err.Error(), `"global"`) || !strings.Contains(err.Error(), "app-synchronizer") {
		t.Fatalf("err = %v, want mention of the global alias and the connected aliases", err)
	}
}

type vettingStub struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []map[string]any
}

func newVettingStub(t *testing.T, pages ...string) *vettingStub {
	t.Helper()
	stub := &vettingStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/parties/participant-id":
			_, _ = io.WriteString(w, `{"participantId":"p1::122a"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/package-vetting/list":
			raw, _ := io.ReadAll(r.Body)
			var decoded map[string]any
			_ = json.Unmarshal(raw, &decoded)
			stub.mu.Lock()
			stub.requests = append(stub.requests, decoded)
			page := len(stub.requests) - 1
			stub.mu.Unlock()
			if page >= len(pages) {
				page = len(pages) - 1
			}
			_, _ = io.WriteString(w, pages[page])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func vettedEntry(participantID, synchronizerID, packageID string) string {
	return `{"packages":[{"packageId":"` + packageID + `"}],"participantId":"` + participantID + `","synchronizerId":"` + synchronizerID + `"}`
}

func TestJsonLedgerAdminClient_IsPackageVetted_FiltersByPackageParticipantAndSynchronizer(t *testing.T) {
	stub := newVettingStub(t, `{"vettedPackages":[`+vettedEntry("p1::122a", "global::122a", "pkg-1")+`]}`)
	client, err := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}

	vetted, err := client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if !vetted {
		t.Error("vetted = false, want true")
	}
	if len(stub.requests) != 1 {
		t.Fatalf("got %d vetting-list requests, want 1", len(stub.requests))
	}
	want := `{"packageMetadataFilter":{"packageIds":["pkg-1"]},"topologyStateFilter":{"participantIds":["p1::122a"],"synchronizerIds":["global::122a"]}}`
	got, _ := json.Marshal(stub.requests[0])
	if string(got) != want {
		t.Errorf("request = %s, want %s", got, want)
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_OmitsTheSynchronizerFilterWhenNoneIsGiven(t *testing.T) {
	stub := newVettingStub(t, `{"vettedPackages":[]}`)
	client, _ := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"})

	if _, err := client.isPackageVetted(context.Background(), "pkg-1", ""); err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	want := `{"packageMetadataFilter":{"packageIds":["pkg-1"]},"topologyStateFilter":{"participantIds":["p1::122a"]}}`
	got, _ := json.Marshal(stub.requests[0])
	if string(got) != want {
		t.Errorf("request = %s, want %s", got, want)
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_IsFalseWhenNoEntryComesBack(t *testing.T) {
	stub := newVettingStub(t, `{"vettedPackages":[]}`)
	client, _ := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"})

	vetted, err := client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if vetted {
		t.Error("vetted = true, want false")
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_IgnoresEntriesOfOtherParticipants(t *testing.T) {
	stub := newVettingStub(t, `{"vettedPackages":[`+vettedEntry("p2::122b", "global::122a", "pkg-1")+`]}`)
	client, _ := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"})

	vetted, err := client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if vetted {
		t.Error("vetted = true for another participant's entry, want false")
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_FollowsNextPageTokenThroughEmptyPages(t *testing.T) {
	stub := newVettingStub(t,
		`{"vettedPackages":[],"nextPageToken":"page-2"}`,
		`{"vettedPackages":[`+vettedEntry("p1::122a", "global::122a", "pkg-1")+`]}`,
	)
	client, _ := NewJsonLedgerAdminClient(stub.server.URL, staticTokenProvider{token: "tok"})

	vetted, err := client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
	if err != nil {
		t.Fatalf("isPackageVetted: %v", err)
	}
	if !vetted {
		t.Error("vetted = false, want true from the second page")
	}
	if len(stub.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(stub.requests))
	}
	if token := stub.requests[1]["pageToken"]; token != "page-2" {
		t.Errorf("second request pageToken = %v, want page-2", token)
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_PropagatesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/parties/participant-id" {
			_, _ = io.WriteString(w, `{"participantId":"p1::122a"}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	t.Cleanup(server.Close)
	client, _ := NewJsonLedgerAdminClient(server.URL, staticTokenProvider{token: "tok"})

	_, err := client.isPackageVetted(context.Background(), "pkg-1", "global::122a")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want one naming 500", err)
	}
}

func TestJsonLedgerAdminClient_IsPackageVetted_RequiresAPackageID(t *testing.T) {
	client, _ := NewJsonLedgerAdminClient("http://localhost", staticTokenProvider{token: "tok"})

	if _, err := client.isPackageVetted(context.Background(), " ", ""); err == nil {
		t.Fatal("expected error for blank packageID")
	}
}
