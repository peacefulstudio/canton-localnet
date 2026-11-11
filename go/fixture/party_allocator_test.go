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
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	partyAlreadyExistsBody   = `{"code":"INVALID_ARGUMENT","cause":"The submitted request has invalid arguments: Party already exists: party globex-abcdef::122034ab... is already allocated on this node"}`
	requestAlreadyInFlight   = `{"code":"REQUEST_ALREADY_IN_FLIGHT","cause":"Request already in flight"}`
	defaultConnectedSyncsRaw = `{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"global::122a"}]}`
)

type allocationStub struct {
	server *httptest.Server

	mu             sync.Mutex
	posts          []partyAllocateRequest
	postAuth       []string
	lookupQueries  []string
	discoveryCalls int
}

func (s *allocationStub) postCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func (s *allocationStub) lookupCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lookupQueries)
}

func newAllocationStub(
	t *testing.T,
	allocation func(attempt int, w http.ResponseWriter),
	lookup func(w http.ResponseWriter),
	connectedSynchronizers ...string,
) *allocationStub {
	t.Helper()
	discovery := defaultConnectedSyncsRaw
	if len(connectedSynchronizers) > 0 {
		discovery = connectedSynchronizers[0]
	}
	stub := &allocationStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/state/connected-synchronizers":
			stub.discoveryCalls++
			_, _ = io.WriteString(w, discovery)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/parties":
			stub.lookupQueries = append(stub.lookupQueries, r.URL.RawQuery)
			lookup(w)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/parties":
			raw, _ := io.ReadAll(r.Body)
			var body partyAllocateRequest
			_ = json.Unmarshal(raw, &body)
			stub.posts = append(stub.posts, body)
			stub.postAuth = append(stub.postAuth, r.Header.Get("Authorization"))
			allocation(len(stub.posts), w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func respondAllocated(w http.ResponseWriter, party string) {
	_, _ = io.WriteString(w, `{"partyDetails":{"party":"`+party+`","isLocal":true}}`)
}

func respondFailure(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func respondKnownParties(parties ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		details := make([]string, 0, len(parties))
		for _, p := range parties {
			details = append(details, `{"party":"`+p+`","isLocal":true}`)
		}
		_, _ = io.WriteString(w, `{"partyDetails":[`+strings.Join(details, ",")+`],"nextPageToken":""}`)
	}
}

func noLookup(w http.ResponseWriter) { respondKnownParties()(w) }

func noSleepRetry(maxAttempts int, delays *[]time.Duration) retryOptions {
	return retryOptions{
		MaxAttempts: maxAttempts,
		BaseDelay:   time.Second,
		MaxDelay:    16 * time.Second,
		Sleep: func(_ context.Context, d time.Duration) error {
			if delays != nil {
				*delays = append(*delays, d)
			}
			return nil
		},
	}
}

func newStubAllocator(t *testing.T, stub *allocationStub, retry retryOptions, extra ...PartyAllocatorOption) *PartyAllocator {
	t.Helper()
	opts := append([]PartyAllocatorOption{WithPartyAllocatorSuffix("abcdef"), withPartyAllocatorRetry(retry)}, extra...)
	p, err := NewPartyAllocator(stub.server.URL, staticTokenProvider{token: "tok"}, opts...)
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	return p
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type timingOutPostTransport struct {
	mu       sync.Mutex
	posts    int
	timeouts int
	inner    http.RoundTripper
}

func (tr *timingOutPostTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost {
		tr.mu.Lock()
		tr.posts++
		timingOut := tr.posts <= tr.timeouts
		tr.mu.Unlock()
		if timingOut {
			return nil, timeoutError{}
		}
	}
	return tr.inner.RoundTrip(r)
}

func (tr *timingOutPostTransport) postCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.posts
}

func allocateStatusError(t *testing.T, err error) *httpStatusError {
	t.Helper()
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *httpStatusError", err)
	}
	return statusErr
}

func TestPartyAllocator_RequestShape(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "alice::ns1") }, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil), WithPartyAllocatorSuffix("abc123"))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "alice::ns1" {
		t.Errorf("party = %q, want alice::ns1", got)
	}
	if stub.postCount() != 1 {
		t.Fatalf("posts = %d, want 1", stub.postCount())
	}
	if stub.posts[0].PartyIDHint != "globex-abc123" {
		t.Errorf("partyIdHint = %q, want globex-abc123", stub.posts[0].PartyIDHint)
	}
	if stub.postAuth[0] != "Bearer tok" {
		t.Errorf("auth = %q", stub.postAuth[0])
	}
}

func TestPartyAllocator_PostCarriesJSONContentTypeAndNoDisplayName(t *testing.T) {
	var contentType, rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, defaultConnectedSyncsRaw)
			return
		}
		contentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		rawBody = string(raw)
		_, _ = io.WriteString(w, `{"partyDetails":{"party":"x::y"}}`)
	}))
	t.Cleanup(server.Close)
	p, err := NewPartyAllocator(server.URL, staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("sfx"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}

	if _, err := p.Allocate(context.Background(), "initech"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if contentType != "application/json" {
		t.Errorf("Content-Type = %q", contentType)
	}
	if strings.Contains(rawBody, "displayName") {
		t.Errorf("body must not carry displayName; body = %s", rawBody)
	}
}

func TestPartyAllocator_DefaultsSynchronizerIDToTheGlobalAliasResolvedViaDiscovery(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "globex-abcdef::namespace") }, noLookup,
		`{"connectedSynchronizers":[`+
			`{"synchronizerAlias":"global","synchronizerId":"global::122a"},`+
			`{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}`)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	if _, err := p.Allocate(context.Background(), "globex"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if stub.posts[0].SynchronizerID != "global::122a" {
		t.Errorf("synchronizerId = %q, want global::122a", stub.posts[0].SynchronizerID)
	}
	if stub.discoveryCalls != 1 {
		t.Errorf("discovery calls = %d, want 1", stub.discoveryCalls)
	}
}

func TestPartyAllocator_AllocateOnSynchronizerWithExplicitIDSkipsDiscoveryAndSendsItVerbatim(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "globex-abcdef::namespace") }, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	if _, err := p.AllocateOnSynchronizer(context.Background(), "globex", "app-synchronizer::122b"); err != nil {
		t.Fatalf("AllocateOnSynchronizer: %v", err)
	}

	if stub.posts[0].SynchronizerID != "app-synchronizer::122b" {
		t.Errorf("synchronizerId = %q, want app-synchronizer::122b", stub.posts[0].SynchronizerID)
	}
	if stub.discoveryCalls != 0 {
		t.Errorf("discovery calls = %d, want 0", stub.discoveryCalls)
	}
}

func TestPartyAllocator_FailsWhenNoGlobalSynchronizerIsConnected(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "x::y") }, noLookup,
		`{"connectedSynchronizers":[{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}`)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	_, err := p.Allocate(context.Background(), "globex")

	if err == nil || !strings.Contains(err.Error(), `"global"`) {
		t.Fatalf("err = %v, want a missing global synchronizer error", err)
	}
	if stub.postCount() != 0 {
		t.Errorf("posts = %d, want 0", stub.postCount())
	}
}

func TestPartyAllocator_RetriesTransient503ThenReturnsTheParty(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusServiceUnavailable, `{"code":"REQUEST_TIME_OUT"}`)
			return
		}
		respondAllocated(w, "globex-abcdef::namespace")
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::namespace" {
		t.Errorf("party = %q", got)
	}
	if stub.postCount() != 2 {
		t.Errorf("posts = %d, want 2", stub.postCount())
	}
}

func TestPartyAllocator_RetriesTransient504ThenReturnsTheParty(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusGatewayTimeout, "")
			return
		}
		respondAllocated(w, "globex-abcdef::namespace")
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::namespace" {
		t.Errorf("party = %q", got)
	}
	if stub.postCount() != 2 {
		t.Errorf("posts = %d, want 2", stub.postCount())
	}
}

func TestPartyAllocator_WaitsOutA409RequestAlreadyInFlightThenAcceptsAlreadyExistsAsSuccess(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		switch attempt {
		case 1:
			respondFailure(w, http.StatusServiceUnavailable, "")
		case 2:
			respondFailure(w, http.StatusConflict, requestAlreadyInFlight)
		default:
			respondFailure(w, http.StatusBadRequest, partyAlreadyExistsBody)
		}
	}, respondKnownParties("globex-abcdef::122034ab5c6d"))
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::122034ab5c6d" {
		t.Errorf("party = %q", got)
	}
	if stub.postCount() != 3 {
		t.Errorf("posts = %d, want 3", stub.postCount())
	}
}

func TestPartyAllocator_RetriesA409RequestAlreadyInFlightOnTheFirstAttempt(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusConflict, requestAlreadyInFlight)
			return
		}
		respondAllocated(w, "globex-abcdef::namespace")
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::namespace" {
		t.Errorf("party = %q", got)
	}
	if stub.postCount() != 2 {
		t.Errorf("posts = %d, want 2", stub.postCount())
	}
}

func TestPartyAllocator_GivesUpAfterTheAttemptBoundOnAPersistent409RequestAlreadyInFlight(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
		respondFailure(w, http.StatusConflict, requestAlreadyInFlight)
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(3, nil))

	_, err := p.Allocate(context.Background(), "globex")

	statusErr := allocateStatusError(t, err)
	if statusErr.statusCode != http.StatusConflict {
		t.Errorf("statusCode = %d, want 409", statusErr.statusCode)
	}
	if !strings.Contains(string(statusErr.body), "REQUEST_ALREADY_IN_FLIGHT") {
		t.Errorf("body = %q", statusErr.body)
	}
	if stub.postCount() != 3 {
		t.Errorf("posts = %d, want 3", stub.postCount())
	}
}

func TestPartyAllocator_BacksOffExponentiallyBetween503Retries(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt <= 3 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondAllocated(w, "globex-abcdef::namespace")
	}, noLookup)
	var delays []time.Duration
	p := newStubAllocator(t, stub, noSleepRetry(6, &delays))

	if _, err := p.Allocate(context.Background(), "globex"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delays[%d] = %v, want %v", i, delays[i], want[i])
		}
	}
}

func TestPartyAllocator_CapsTheBackoffAtMaxDelay(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt <= 6 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondAllocated(w, "globex-abcdef::namespace")
	}, noLookup)
	var delays []time.Duration
	p := newStubAllocator(t, stub, noSleepRetry(8, &delays))

	if _, err := p.Allocate(context.Background(), "globex"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 16 * time.Second}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delays[%d] = %v, want %v", i, delays[i], want[i])
		}
	}
}

func TestPartyAllocator_RetriesARequestTimeoutThenReturnsTheParty(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "globex-abcdef::namespace") }, noLookup)
	transport := &timingOutPostTransport{timeouts: 1, inner: http.DefaultTransport}
	p := newStubAllocator(t, stub, noSleepRetry(6, nil), WithPartyAllocatorHTTPClient(&http.Client{Transport: transport}))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::namespace" {
		t.Errorf("party = %q", got)
	}
	if transport.postCount() != 2 {
		t.Errorf("posts = %d, want 2", transport.postCount())
	}
}

func TestPartyAllocator_ReturnsTheExistingPartyWhenARetryFindsItAlreadyAllocated(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondFailure(w, http.StatusBadRequest, partyAlreadyExistsBody)
	}, respondKnownParties("globex-abcdef::122034ab5c6d"))
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::122034ab5c6d" {
		t.Errorf("party = %q", got)
	}
	if stub.postCount() != 2 {
		t.Errorf("posts = %d, want 2", stub.postCount())
	}
}

func TestPartyAllocator_LooksThePartyUpByHintPrefixWhenARetryFindsItAlreadyAllocated(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondFailure(w, http.StatusBadRequest, partyAlreadyExistsBody)
	}, respondKnownParties("globex-abcdef2::122034ffff", "globex-abcdef::122034ab5c6d"))
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	got, err := p.Allocate(context.Background(), "globex")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != "globex-abcdef::122034ab5c6d" {
		t.Errorf("party = %q", got)
	}
	if stub.lookupCount() != 1 || stub.lookupQueries[0] != "filter-party=globex-abcdef%3A%3A" {
		t.Errorf("lookup queries = %v, want [filter-party=globex-abcdef%%3A%%3A]", stub.lookupQueries)
	}
}

func TestPartyAllocator_GivesUpAfterTheAttemptBoundOnPersistent503(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
		respondFailure(w, http.StatusServiceUnavailable, `{"code":"REQUEST_TIME_OUT"}`)
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(3, nil))

	_, err := p.Allocate(context.Background(), "globex")

	statusErr := allocateStatusError(t, err)
	if statusErr.statusCode != http.StatusServiceUnavailable {
		t.Errorf("statusCode = %d, want 503", statusErr.statusCode)
	}
	if !strings.Contains(err.Error(), "REQUEST_TIME_OUT") || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("err = %q, want status, body and attempt count", err.Error())
	}
	if stub.postCount() != 3 {
		t.Errorf("posts = %d, want 3", stub.postCount())
	}
}

func TestPartyAllocator_GivesUpAfterTheAttemptBoundOnPersistentRequestTimeout(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "x::y") }, noLookup)
	transport := &timingOutPostTransport{timeouts: 100, inner: http.DefaultTransport}
	p := newStubAllocator(t, stub, noSleepRetry(3, nil), WithPartyAllocatorHTTPClient(&http.Client{Transport: transport}))

	_, err := p.Allocate(context.Background(), "globex")

	if err == nil || !strings.Contains(err.Error(), "3 attempts") || !strings.Contains(err.Error(), "globex-abcdef") {
		t.Fatalf("err = %v, want attempt count and hint", err)
	}
	var timeout interface{ Timeout() bool }
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Errorf("err = %v, want it to wrap the timeout", err)
	}
	if transport.postCount() != 3 {
		t.Errorf("posts = %d, want 3", transport.postCount())
	}
}

func TestPartyAllocator_DoesNotRetryANonTransientError(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusConflict,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
				respondFailure(w, status, `{"code":"INVALID_ARGUMENT"}`)
			}, noLookup)
			p := newStubAllocator(t, stub, noSleepRetry(6, nil))

			_, err := p.Allocate(context.Background(), "globex")

			if got := allocateStatusError(t, err).statusCode; got != status {
				t.Errorf("statusCode = %d, want %d", got, status)
			}
			if stub.postCount() != 1 {
				t.Errorf("posts = %d, want 1", stub.postCount())
			}
		})
	}
}

func TestPartyAllocator_DoesNotTreatAlreadyExistsOnTheFirstAttemptAsSuccess(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
		respondFailure(w, http.StatusBadRequest, partyAlreadyExistsBody)
	}, respondKnownParties("globex-abcdef::122034ab5c6d"))
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	_, err := p.Allocate(context.Background(), "globex")

	statusErr := allocateStatusError(t, err)
	if statusErr.statusCode != http.StatusBadRequest || !strings.Contains(string(statusErr.body), "Party already exists") {
		t.Errorf("err = %v, want the 400 already-exists error", err)
	}
	if stub.lookupCount() != 0 {
		t.Errorf("lookups = %d, want 0", stub.lookupCount())
	}
}

func TestPartyAllocator_SurfacesTheAlreadyExistsErrorWhenTheLookupFindsNoParty(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondFailure(w, http.StatusBadRequest, partyAlreadyExistsBody)
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	_, err := p.Allocate(context.Background(), "globex")

	statusErr := allocateStatusError(t, err)
	if statusErr.statusCode != http.StatusBadRequest || !strings.Contains(string(statusErr.body), "Party already exists") {
		t.Errorf("err = %v, want the 400 already-exists error", err)
	}
}

func TestPartyAllocator_HintFollowsPrefixSuffixFormat(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("ZZZ"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if got := p.Hint("initech"); got != "initech-ZZZ" {
		t.Errorf("Hint(initech) = %q, want initech-ZZZ", got)
	}
	if got := p.Hint("globex"); got != "globex-ZZZ" {
		t.Errorf("Hint(globex) = %q, want globex-ZZZ", got)
	}
	if p.Suffix() != "ZZZ" {
		t.Errorf("Suffix = %q, want ZZZ", p.Suffix())
	}
}

func TestPartyAllocator_GeneratesUniqueSuffixPerInstance(t *testing.T) {
	a, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if a.Suffix() == "" || b.Suffix() == "" {
		t.Fatal("expected non-empty suffixes")
	}
	if a.Suffix() == b.Suffix() {
		t.Fatalf("suffixes should differ between instances: a=%q b=%q", a.Suffix(), b.Suffix())
	}
	if len(a.Suffix()) < 8 {
		t.Errorf("suffix %q is suspiciously short (need at least 8 hex chars)", a.Suffix())
	}
}

func TestPartyAllocator_SuffixIsStableAcrossCalls(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	first := p.Hint("globex")
	for i := 0; i < 5; i++ {
		if p.Hint("globex") != first {
			t.Fatalf("hint changed across calls: %q vs %q", first, p.Hint("globex"))
		}
	}
}

func TestPartyAllocator_PropagatesNon2xx(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
		respondFailure(w, http.StatusConflict, `{"cause":"PARTY_ALREADY_EXISTS"}`)
	}, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	_, err := p.Allocate(context.Background(), "initech")

	if err == nil {
		t.Fatal("expected error on 409")
	}
	if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "PARTY_ALREADY_EXISTS") {
		t.Errorf("error %q missing status or cause", err.Error())
	}
}

func TestPartyAllocator_RejectsEmptyPartyInResponse(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { respondAllocated(w, "") }, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	if _, err := p.Allocate(context.Background(), "initech"); err == nil {
		t.Fatal("expected error for empty party in response")
	}
}

func TestPartyAllocator_RejectsInvalidJSON(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) { _, _ = io.WriteString(w, `not json`) }, noLookup)
	p := newStubAllocator(t, stub, noSleepRetry(6, nil))

	if _, err := p.Allocate(context.Background(), "initech"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestPartyAllocator_PropagatesTokenError(t *testing.T) {
	p, err := NewPartyAllocator("http://localhost", staticTokenProvider{err: errors.New("no token")}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	if _, err := p.Allocate(context.Background(), "initech"); err == nil {
		t.Fatal("expected token error")
	}
}

func TestPartyAllocator_HonorsContextCancel(t *testing.T) {
	p, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"}, WithPartyAllocatorSuffix("s"))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Allocate(ctx, "initech"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Allocate(cancelled) = %v, want context.Canceled", err)
	}
}

func TestPartyAllocator_RequiresArguments(t *testing.T) {
	if _, err := NewPartyAllocator("", staticTokenProvider{token: "tok"}); err == nil {
		t.Error("expected error for empty baseURL")
	}
	if _, err := NewPartyAllocator("http://x", nil); err == nil {
		t.Error("expected error for nil tokens")
	}
	p, _ := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"})
	if _, err := p.Allocate(context.Background(), ""); err == nil {
		t.Error("expected error for empty prefix")
	}
}

func TestPartyAllocator_StopsRetryingWhenTheContextIsCancelledDuringBackoff(t *testing.T) {
	stub := newAllocationStub(t, func(_ int, w http.ResponseWriter) {
		respondFailure(w, http.StatusServiceUnavailable, "")
	}, noLookup)
	retry := noSleepRetry(6, nil)
	ctx, cancel := context.WithCancel(context.Background())
	retry.Sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	p := newStubAllocator(t, stub, retry)

	_, err := p.Allocate(ctx, "globex")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if stub.postCount() != 1 {
		t.Errorf("posts = %d, want 1", stub.postCount())
	}
}

func TestPartyAllocator_RejectsInvalidRetryOptions(t *testing.T) {
	sleep := func(context.Context, time.Duration) error { return nil }
	cases := map[string]retryOptions{
		"zero attempts":        {MaxAttempts: 0, BaseDelay: time.Second, MaxDelay: time.Second, Sleep: sleep},
		"negative base delay":  {MaxAttempts: 1, BaseDelay: -time.Second, MaxDelay: time.Second, Sleep: sleep},
		"max below base delay": {MaxAttempts: 1, BaseDelay: 2 * time.Second, MaxDelay: time.Second, Sleep: sleep},
		"missing sleep":        {MaxAttempts: 1, BaseDelay: time.Second, MaxDelay: time.Second},
	}
	for name, retry := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPartyAllocator("http://x", staticTokenProvider{token: "tok"}, withPartyAllocatorRetry(retry)); err == nil {
				t.Error("expected error for invalid retry options")
			}
		})
	}
}

func TestDefaultRetryOptions_AreSixAttemptsOneSecondBaseSixteenSecondCap(t *testing.T) {
	got := defaultRetryOptions()

	if got.MaxAttempts != 6 || got.BaseDelay != time.Second || got.MaxDelay != 16*time.Second || got.Sleep == nil {
		t.Errorf("defaultRetryOptions() = %+v", got)
	}
}

func TestPartyAllocator_AcquiresATokenForEachAttempt(t *testing.T) {
	stub := newAllocationStub(t, func(attempt int, w http.ResponseWriter) {
		if attempt == 1 {
			respondFailure(w, http.StatusServiceUnavailable, "")
			return
		}
		respondAllocated(w, "globex-abcdef::122034")
	}, noLookup)
	p, err := NewPartyAllocator(stub.server.URL, &countingTokenProvider{},
		WithPartyAllocatorSuffix("abcdef"), withPartyAllocatorRetry(noSleepRetry(6, nil)))
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}

	if _, err := p.Allocate(context.Background(), "globex"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.postAuth) != 2 {
		t.Fatalf("posts = %d, want 2", len(stub.postAuth))
	}
	if stub.postAuth[0] == stub.postAuth[1] {
		t.Errorf("both attempts sent %q; each attempt must acquire its own token", stub.postAuth[0])
	}
}
