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
	"fmt"
	"net/http"
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
// Setup is idempotent. Teardown invalidates the cached token and resets
// the fixture so a subsequent Setup re-establishes credentials.
type Fixture struct {
	cfg       Config
	endpoints Endpoints
	tokens    *OAuth2TokenProvider
	admin     *JsonLedgerAdminClient
	dars      *DarUploader
	parties   *PartyAllocator
	users     *UserBuilder
	ready     bool
}

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
	parties, err := NewPartyAllocator(endpoints.JSONLedgerAPIURL, tokens, WithPartyAllocatorHTTPClient(f.cfg.HTTPClient))
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

// Teardown invalidates the cached access token and resets the fixture so
// the next Setup re-establishes credentials. It honours ctx cancellation
// at entry; further blocking I/O added by future maintainers should also
// observe ctx.
func (f *Fixture) Teardown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !f.ready {
		return nil
	}
	f.tokens.Invalidate()
	f.dars = nil
	f.parties = nil
	f.users = nil
	f.ready = false
	return nil
}

// Endpoints returns the resolved endpoint set. Panics if called before Setup.
func (f *Fixture) Endpoints() Endpoints {
	f.mustBeReady("Endpoints")
	return f.endpoints
}

// Tokens returns the OAuth2 token provider. Panics if called before Setup.
func (f *Fixture) Tokens() *OAuth2TokenProvider {
	f.mustBeReady("Tokens")
	return f.tokens
}

// Admin returns the JSON Ledger Admin client. Panics if called before Setup.
func (f *Fixture) Admin() *JsonLedgerAdminClient {
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

// DarUploader returns the DAR uploader. Panics if called before Setup.
func (f *Fixture) DarUploader() *DarUploader {
	f.mustBeReady("DarUploader")
	return f.dars
}

// PartyAllocator returns the party allocator. Panics if called before Setup.
func (f *Fixture) PartyAllocator() *PartyAllocator {
	f.mustBeReady("PartyAllocator")
	return f.parties
}

// UserBuilder returns the user builder. Panics if called before Setup.
func (f *Fixture) UserBuilder() *UserBuilder {
	f.mustBeReady("UserBuilder")
	return f.users
}

// UploadDar uploads a single DAR file via the JSON Ledger packages endpoint.
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
func (f *Fixture) AllocateParty(ctx context.Context, prefix, displayName string) (string, error) {
	return f.PartyAllocator().Allocate(ctx, prefix, displayName)
}

// CreateUser creates a ledger user and grants any ActAs / ReadAs rights
// listed in opts. Convenience wrapper around UserBuilder().Create.
func (f *Fixture) CreateUser(ctx context.Context, opts UserOptions) (string, error) {
	return f.UserBuilder().Create(ctx, opts)
}
