// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"fmt"
)

// ValidatorFixture is a per-slot view of a Fixture. It exposes the same
// deep modules as Fixture (admin client, DAR uploader, party allocator,
// user builder) but scoped to a single validator slot, so tests that
// span multiple validators don't have to juggle separate top-level
// fixtures.
//
// Obtain one by calling Fixture.Validator(role). The first call for a
// given role resolves endpoints and constructs the underlying clients;
// subsequent calls return the cached instance.
type ValidatorFixture struct {
	role      Role
	endpoints Endpoints
	tokens    *OAuth2TokenProvider
	admin     *JsonLedgerAdminClient
	dars      *DarUploader
	parties   *PartyAllocator
	users     *UserBuilder
}

// Role returns the slot this view is bound to.
func (v *ValidatorFixture) Role() Role { return v.role }

// Endpoints returns the resolved endpoint set for this validator.
func (v *ValidatorFixture) Endpoints() Endpoints { return v.endpoints }

// Tokens returns the OAuth2 token provider scoped to this validator's realm.
func (v *ValidatorFixture) Tokens() *OAuth2TokenProvider { return v.tokens }

// Admin returns the JSON Ledger Admin client bound to this validator.
func (v *ValidatorFixture) Admin() *JsonLedgerAdminClient { return v.admin }

// DarUploader returns the DAR uploader bound to this validator.
func (v *ValidatorFixture) DarUploader() *DarUploader { return v.dars }

// PartyAllocator returns the party allocator bound to this validator.
func (v *ValidatorFixture) PartyAllocator() *PartyAllocator { return v.parties }

// UserBuilder returns the user builder bound to this validator.
func (v *ValidatorFixture) UserBuilder() *UserBuilder { return v.users }

// GetParticipantId is a convenience wrapper around Admin().GetParticipantId.
func (v *ValidatorFixture) GetParticipantId(ctx context.Context) (string, error) {
	return v.admin.GetParticipantId(ctx)
}

// UploadDar uploads a single DAR via this validator's JSON Ledger API.
func (v *ValidatorFixture) UploadDar(ctx context.Context, path string) error {
	return v.dars.Upload(ctx, path)
}

// UploadDars uploads several DARs sequentially via this validator's JSON
// Ledger API. Stops at the first failure.
func (v *ValidatorFixture) UploadDars(ctx context.Context, paths ...string) error {
	return v.dars.UploadAll(ctx, paths...)
}

// AllocateParty allocates a party with hint "<prefix>-<random suffix>"
// against this validator.
func (v *ValidatorFixture) AllocateParty(ctx context.Context, prefix, displayName string) (string, error) {
	return v.parties.Allocate(ctx, prefix, displayName)
}

// CreateUser creates a ledger user on this validator and grants
// ActAs/ReadAs rights from opts.
func (v *ValidatorFixture) CreateUser(ctx context.Context, opts UserOptions) (string, error) {
	return v.users.Create(ctx, opts)
}

// Validator returns a per-role view of the fixture. The first call for
// each role resolves that role's endpoints and constructs scoped admin
// / DAR / party / user clients; subsequent calls return the cached
// instance. Setup must have been called against the fixture first so
// the shared HTTPClient is in place; the returned ValidatorFixture
// reuses that HTTPClient.
//
// Passing the same Role the fixture was constructed with returns a view
// backed by the same clients as Admin() / DarUploader() / etc., so
// existing code that mixes the two surfaces stays consistent.
func (f *Fixture) Validator(role Role) (*ValidatorFixture, error) {
	if !f.ready {
		return nil, fmt.Errorf("fixture: Setup must be called before Validator")
	}
	f.validatorsMu.Lock()
	defer f.validatorsMu.Unlock()
	if existing, ok := f.validators[role]; ok {
		return existing, nil
	}
	v, err := f.buildValidator(role)
	if err != nil {
		return nil, err
	}
	if f.validators == nil {
		f.validators = map[Role]*ValidatorFixture{}
	}
	f.validators[role] = v
	return v, nil
}

// MustValidator is the panicking counterpart to Validator, intended for
// test bodies where an error in endpoint resolution should fail the
// test outright. Callers that want to surface the error gracefully
// should use Validator instead.
func (f *Fixture) MustValidator(role Role) *ValidatorFixture {
	v, err := f.Validator(role)
	if err != nil {
		panic("fixture: Validator(" + string(role) + "): " + err.Error())
	}
	return v
}

func (f *Fixture) buildValidator(role Role) (*ValidatorFixture, error) {
	if role == f.cfg.Role {
		return &ValidatorFixture{
			role:      role,
			endpoints: f.endpoints,
			tokens:    f.tokens,
			admin:     f.admin,
			dars:      f.dars,
			parties:   f.parties,
			users:     f.users,
		}, nil
	}
	endpoints, err := f.cfg.Discovery.For(role)
	if err != nil {
		return nil, fmt.Errorf("fixture: discover endpoints for %s: %w", role, err)
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
		return nil, fmt.Errorf("fixture: build token provider for %s: %w", role, err)
	}
	admin, err := NewJsonLedgerAdminClient(endpoints.JSONLedgerAPIURL, tokens, WithHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return nil, fmt.Errorf("fixture: build admin client for %s: %w", role, err)
	}
	dars, err := NewDarUploader(endpoints.JSONLedgerAPIURL, tokens, WithDarHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return nil, fmt.Errorf("fixture: build dar uploader for %s: %w", role, err)
	}
	parties, err := NewPartyAllocator(endpoints.JSONLedgerAPIURL, tokens, WithPartyAllocatorHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return nil, fmt.Errorf("fixture: build party allocator for %s: %w", role, err)
	}
	users, err := NewUserBuilder(endpoints.JSONLedgerAPIURL, tokens, WithUserBuilderHTTPClient(f.cfg.HTTPClient))
	if err != nil {
		return nil, fmt.Errorf("fixture: build user builder for %s: %w", role, err)
	}
	return &ValidatorFixture{
		role:      role,
		endpoints: endpoints,
		tokens:    tokens,
		admin:     admin,
		dars:      dars,
		parties:   parties,
		users:     users,
	}, nil
}

// KnownRoles returns the full slot set the fixture knows about, in a
// stable order: sv, a, b, c, d. Useful for tests that want to iterate
// every validator.
func KnownRoles() []Role {
	return []Role{
		RoleSvValidator1,
		RoleAValidator1,
		RoleBValidator1,
		RoleCValidator1,
		RoleDValidator1,
	}
}

