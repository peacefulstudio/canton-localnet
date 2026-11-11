// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package fixture provides a Go-side integration-test harness for the Canton
// LocalNet compose stack. It exposes endpoint discovery, an OAuth2
// client-credentials token provider with cached refresh, and a minimal JSON
// Ledger Admin API client.
//
// The Fixture type composes these primitives behind a Setup / Teardown
// surface that's deliberately free of test-framework dependencies — call it
// from `go test`, from a `TestMain`, or from a `main` smoke binary.
package fixture

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Config configures a Fixture. All fields are optional and have sensible
// defaults; in particular, an empty Role defaults to RoleAValidator1, and a
// nil Discovery defaults to NewEndpointDiscovery (env-based).
type Config struct {
	Role       Role
	Discovery  *EndpointDiscovery
	HTTPClient *http.Client
	Now        func() time.Time
}

// Fixture wires endpoint discovery, an OAuth2 token provider and a JSON
// Ledger Admin client into a single struct ready for integration tests.
//
// Lifecycle:
//
//	f, err := fixture.New(fixture.Config{Role: fixture.RoleAValidator1})
//	if err != nil { t.Fatal(err) }
//	if err := f.Setup(ctx); err != nil { t.Fatal(err) }
//	t.Cleanup(func() { _ = f.Teardown(context.Background()) })
//
// Setup is idempotent. Teardown deletes the ledger users created through
// CreateUser, invalidates the cached token and resets the fixture so a
// subsequent Setup re-establishes credentials.
type Fixture struct {
	cfg          Config
	endpoints    Endpoints
	tokens       *OAuth2TokenProvider
	admin        *JsonLedgerAdminClient
	dars         *DarUploader
	parties      *PartyAllocator
	users        *UserBuilder
	created      createdUsers
	mu           sync.RWMutex
	ready        bool
	validatorsMu sync.Mutex
	validators   map[Role]*ValidatorFixture
}

// String formats the fixture with its endpoints' ClientSecret redacted, so
// logging a Fixture never writes the secret.
func (f *Fixture) String() string {
	return fmt.Sprintf("Fixture{Endpoints:%s}", f.endpoints.String())
}

// GoString implements fmt.GoStringer so %#v redacts ClientSecret as String does.
func (f *Fixture) GoString() string { return f.String() }

// New constructs a Fixture from the supplied Config. It does not perform any
// network I/O; call Setup before issuing requests.
func New(cfg Config) (*Fixture, error) {
	if cfg.Role == "" {
		cfg.Role = RoleAValidator1
	}
	if cfg.Discovery == nil {
		cfg.Discovery = NewEndpointDiscovery()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Fixture{cfg: cfg}, nil
}

// Setup resolves endpoints and constructs the token provider and admin client.
// Calling Setup more than once is a no-op after the first successful invocation.
func (f *Fixture) Setup(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ready {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	endpoints, err := f.cfg.Discovery.For(f.cfg.Role)
	if err != nil {
		return fmt.Errorf("fixture: discover endpoints: %w", err)
	}
	tokens, err := NewOAuth2TokenProvider(OAuth2TokenProviderConfig{
		TokenURL:     endpoints.TokenURL,
		ClientID:     endpoints.ClientID,
		ClientSecret: endpoints.ClientSecret,
		Audience:     endpoints.Audience,
		Scope:        endpoints.Scope,
		HTTPClient:   f.cfg.HTTPClient,
		Now:          f.cfg.Now,
	})
	if err != nil {
		return fmt.Errorf("fixture: build token provider: %w", err)
	}
	admin, err := NewJsonLedgerAdminClient(endpoints.JSONLedgerAPIURL, tokens, WithHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return fmt.Errorf("fixture: build admin client: %w", err)
	}
	dars, err := NewDarUploader(endpoints.JSONLedgerAPIURL, tokens, WithDarHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return fmt.Errorf("fixture: build dar uploader: %w", err)
	}
	parties, err := NewPartyAllocator(endpoints.JSONLedgerAPIURL, tokens, WithPartyAllocatorHTTPClient(f.cfg.HTTPClient), withPartyAllocatorAdminClient(admin))
	if err != nil {
		return fmt.Errorf("fixture: build party allocator: %w", err)
	}
	users, err := NewUserBuilder(endpoints.JSONLedgerAPIURL, tokens, WithUserBuilderHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return fmt.Errorf("fixture: build user builder: %w", err)
	}
	f.endpoints = endpoints
	f.tokens = tokens
	f.admin = admin
	f.dars = dars
	f.parties = parties
	f.users = users
	f.ready = true
	return nil
}

// Teardown deletes the ledger users created through CreateUser on this
// fixture and on every Validator view, then invalidates the cached access
// token and resets the fixture so the next Setup re-establishes credentials.
//
// Each user costs one DELETE /v2/users/{id} per attempt and no GET. The first
// attempt returns an error for any non-2xx response, so a user left behind on
// a shared participant is never silent. A delete that fails does not stop the
// others, and the error is returned once every user has had its attempt. The
// fixture then stays set up: call Teardown again to retry only the users still
// outstanding. From the second attempt on, the participant's 404
// USER_NOT_FOUND counts as success. It honours ctx cancellation at entry and
// on every delete.
func (f *Fixture) Teardown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ready {
		return nil
	}
	if err := f.deleteCreatedUsers(ctx); err != nil {
		return err
	}
	f.tokens.Invalidate()
	f.dars = nil
	f.parties = nil
	f.users = nil
	f.validatorsMu.Lock()
	for _, v := range f.validators {
		if v.tokens != nil && v.tokens != f.tokens {
			v.tokens.Invalidate()
		}
	}
	f.validators = nil
	f.validatorsMu.Unlock()
	f.ready = false
	return nil
}

func (f *Fixture) deleteCreatedUsers(ctx context.Context) error {
	f.validatorsMu.Lock()
	views := make([]*ValidatorFixture, 0, len(f.validators))
	for _, v := range f.validators {
		views = append(views, v)
	}
	f.validatorsMu.Unlock()

	var failures []error
	for _, v := range views {
		failures = append(failures, v.created.deleteAll(ctx, v.users))
	}
	failures = append(failures, f.created.deleteAll(ctx, f.users))
	return errors.Join(failures...)
}

// Endpoints returns the resolved endpoint set. Panics if called before Setup.
func (f *Fixture) Endpoints() Endpoints {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("Endpoints")
	return f.endpoints
}

// Tokens returns the OAuth2 token provider. Panics if called before Setup.
func (f *Fixture) Tokens() *OAuth2TokenProvider {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("Tokens")
	return f.tokens
}

// Admin returns the JSON Ledger Admin client. Panics if called before Setup.
func (f *Fixture) Admin() *JsonLedgerAdminClient {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("Admin")
	return f.admin
}

func (f *Fixture) mustBeReady(method string) {
	if !f.ready {
		panic("fixture: Setup must be called before " + method)
	}
}

// GetParticipantId is a convenience wrapper around Admin().GetParticipantId.
// Panics if called before Setup, consistent with the other accessors.
func (f *Fixture) GetParticipantId(ctx context.Context) (string, error) {
	return f.Admin().GetParticipantId(ctx)
}

// GetConnectedSynchronizers is a convenience wrapper around Admin().GetConnectedSynchronizers.
func (f *Fixture) GetConnectedSynchronizers(ctx context.Context, party string) ([]ConnectedSynchronizer, error) {
	return f.Admin().GetConnectedSynchronizers(ctx, party)
}

// GetAppSynchronizerId is a convenience wrapper around Admin().GetAppSynchronizerId.
func (f *Fixture) GetAppSynchronizerId(ctx context.Context, party string) (string, error) {
	return f.Admin().GetAppSynchronizerId(ctx, party)
}

// DarUploader returns the DAR uploader. Panics if called before Setup.
func (f *Fixture) DarUploader() *DarUploader {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("DarUploader")
	return f.dars
}

// PartyAllocator returns the party allocator. Panics if called before Setup.
func (f *Fixture) PartyAllocator() *PartyAllocator {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("PartyAllocator")
	return f.parties
}

// UserBuilder returns the user builder. Panics if called before Setup.
func (f *Fixture) UserBuilder() *UserBuilder {
	f.mu.RLock()
	defer f.mu.RUnlock()
	f.mustBeReady("UserBuilder")
	return f.users
}

// UploadDar uploads a single DAR file via the JSON Ledger dars endpoint.
// Convenience wrapper around DarUploader().Upload.
func (f *Fixture) UploadDar(ctx context.Context, path string) error {
	return f.DarUploader().Upload(ctx, path)
}

// UploadDars uploads multiple DAR files sequentially. Stops at the first
// failure. Convenience wrapper around DarUploader().UploadAll.
func (f *Fixture) UploadDars(ctx context.Context, paths ...string) error {
	return f.DarUploader().UploadAll(ctx, paths...)
}

// AllocateParty allocates a party with a hint of "<prefix>-<random suffix>"
// and returns the allocated party id. Convenience wrapper around
// PartyAllocator().Allocate.
func (f *Fixture) AllocateParty(ctx context.Context, prefix string) (string, error) {
	return f.PartyAllocator().Allocate(ctx, prefix)
}

// CreateUser creates a ledger user and grants any ActAs / ReadAs rights
// listed in opts. Convenience wrapper around UserBuilder().Create that also
// records the user, so Teardown deletes it.
func (f *Fixture) CreateUser(ctx context.Context, opts UserOptions) (string, error) {
	id, err := f.UserBuilder().Create(ctx, opts)
	if err != nil {
		return "", err
	}
	f.created.record(id)
	return id, nil
}
