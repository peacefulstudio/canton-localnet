// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	allocatePath             = "/v2/parties"
	partyAlreadyExistsMarker = "Party already exists"
)

// PartyAllocator allocates parties via POST /v2/parties.
//
// Each allocator instance generates a random hex suffix at construction
// time; allocate requests send a hint of the form "<consumer-prefix>-<suffix>"
// so that two test runs against the same ledger never collide on a party id.
// The suffix is stable across calls on the same instance, which keeps logs
// readable when several parties share a test.
//
// Unless the caller names a synchronizer (AllocateOnSynchronizer), the party
// is allocated on the global synchronizer, resolved by alias, so allocation
// works unchanged on single-sync and multi-sync stacks. A transient 503 or
// 504, a 409 REQUEST_ALREADY_IN_FLIGHT or a request timeout is retried with
// bounded backoff; when an earlier attempt may have committed, a 400 "Party
// already exists" is resolved by looking the party up by its hint.
type PartyAllocator struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
	suffix     string
	admin      *JsonLedgerAdminClient
	retry      retryOptions
}

// PartyAllocatorOption mutates a PartyAllocator at construction.
type PartyAllocatorOption func(*PartyAllocator)

// WithPartyAllocatorHTTPClient overrides the default HTTP client.
func WithPartyAllocatorHTTPClient(c *http.Client) PartyAllocatorOption {
	return func(p *PartyAllocator) {
		if c != nil {
			p.httpClient = c
		}
	}
}

// WithPartyAllocatorSuffix overrides the random instance suffix. Intended
// for tests that need deterministic hints; production callers should let
// NewPartyAllocator generate one.
func WithPartyAllocatorSuffix(suffix string) PartyAllocatorOption {
	return func(p *PartyAllocator) {
		if suffix != "" {
			p.suffix = suffix
		}
	}
}

// withPartyAllocatorAdminClient sets the admin client used for synchronizer
// discovery. By default the allocator builds its own over the same base URL,
// token provider and HTTP client.
func withPartyAllocatorAdminClient(admin *JsonLedgerAdminClient) PartyAllocatorOption {
	return func(p *PartyAllocator) {
		if admin != nil {
			p.admin = admin
		}
	}
}

// withPartyAllocatorRetry overrides the retry policy; the default is
// defaultRetryOptions. NewPartyAllocator rejects an invalid policy.
func withPartyAllocatorRetry(retry retryOptions) PartyAllocatorOption {
	return func(p *PartyAllocator) {
		p.retry = retry
	}
}

// NewPartyAllocator constructs a PartyAllocator.
func NewPartyAllocator(baseURL string, tokens TokenProvider, opts ...PartyAllocatorOption) (*PartyAllocator, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("party: baseURL is required")
	}
	if tokens == nil {
		return nil, errors.New("party: TokenProvider is required")
	}
	suffix, err := randomSuffix(8)
	if err != nil {
		return nil, fmt.Errorf("party: generate suffix: %w", err)
	}
	p := &PartyAllocator{
		baseURL:    normalizeBaseURL(baseURL),
		tokens:     tokens,
		httpClient: newDefaultHTTPClient(),
		suffix:     suffix,
		retry:      defaultRetryOptions(),
	}
	for _, opt := range opts {
		opt(p)
	}
	if err := p.retry.validate(); err != nil {
		return nil, fmt.Errorf("party: %w", err)
	}
	if p.admin == nil {
		admin, err := NewJsonLedgerAdminClient(p.baseURL, p.tokens, WithHTTPClient(p.httpClient))
		if err != nil {
			return nil, fmt.Errorf("party: build admin client: %w", err)
		}
		p.admin = admin
	}
	return p, nil
}

// Suffix returns the random per-instance suffix used to disambiguate
// party hints.
func (p *PartyAllocator) Suffix() string {
	return p.suffix
}

// Hint returns the hint that would be sent for the supplied consumer prefix.
// Useful for assertions and for logging the planned party id ahead of the
// allocate call.
func (p *PartyAllocator) Hint(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return p.suffix
	}
	return prefix + "-" + p.suffix
}

type partyAllocateRequest struct {
	PartyIDHint        string `json:"partyIdHint"`
	IdentityProviderID string `json:"identityProviderId"`
	SynchronizerID     string `json:"synchronizerId"`
}

type partyDetails struct {
	Party string `json:"party"`
}

type partyAllocateResponse struct {
	PartyDetails partyDetails `json:"partyDetails"`
}

type listKnownPartiesResponse struct {
	PartyDetails []partyDetails `json:"partyDetails"`
}

// Allocate posts a party-allocation request with hint "<prefix>-<suffix>"
// on the global synchronizer, resolved by GlobalSynchronizerAlias, and
// returns the allocated party id from partyDetails.party.
func (p *PartyAllocator) Allocate(ctx context.Context, prefix string) (string, error) {
	return p.AllocateOnSynchronizer(ctx, prefix, "")
}

// AllocateOnSynchronizer is Allocate on synchronizerID instead of the global
// synchronizer default; an empty synchronizerID resolves the global
// synchronizer by alias.
func (p *PartyAllocator) AllocateOnSynchronizer(ctx context.Context, prefix, synchronizerID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(prefix) == "" {
		return "", errors.New("party: prefix is required")
	}
	if strings.TrimSpace(synchronizerID) == "" {
		resolved, err := p.admin.GetGlobalSynchronizerId(ctx)
		if err != nil {
			return "", fmt.Errorf("party: resolve global synchronizer: %w", err)
		}
		synchronizerID = resolved
	}
	hint := p.Hint(prefix)
	raw, err := json.Marshal(partyAllocateRequest{
		PartyIDHint:        hint,
		IdentityProviderID: "",
		SynchronizerID:     synchronizerID,
	})
	if err != nil {
		return "", fmt.Errorf("party: marshal request: %w", err)
	}
	return p.postWithRetry(ctx, hint, raw)
}

func (p *PartyAllocator) postWithRetry(ctx context.Context, hint string, payload []byte) (string, error) {
	earlierAttemptInconclusive := false
	for attempt := 1; ; attempt++ {
		body, err := doRequest(ctx, p.httpClient, p.tokens, p.baseURL, httpRequest{
			method:      http.MethodPost,
			path:        allocatePath,
			body:        payload,
			contentType: "application/json",
			errPrefix:   "party",
		})
		if err == nil {
			return decodeAllocatedParty(body)
		}
		if ctx.Err() != nil {
			return "", err
		}

		var statusErr *httpStatusError
		switch {
		case errors.As(err, &statusErr) && isTransientStatus(statusErr):
			if attempt >= p.retry.MaxAttempts {
				return "", fmt.Errorf("party: POST %s (hint=%s) failed on all %d attempts: %w", allocatePath, hint, p.retry.MaxAttempts, err)
			}
		case isRequestTimeout(err):
			if attempt >= p.retry.MaxAttempts {
				return "", fmt.Errorf("party: POST %s (hint=%s) timed out on all %d attempts: %w", allocatePath, hint, p.retry.MaxAttempts, err)
			}
		case earlierAttemptInconclusive && statusErr != nil && isPartyAlreadyExists(statusErr):
			existing, lookupErr := p.findPartyByHint(ctx, hint)
			if lookupErr != nil {
				return "", lookupErr
			}
			if existing != "" {
				return existing, nil
			}
			return "", err
		default:
			return "", err
		}

		earlierAttemptInconclusive = true
		if sleepErr := p.retry.Sleep(ctx, p.retry.delayForAttempt(attempt)); sleepErr != nil {
			return "", sleepErr
		}
	}
}

func (p *PartyAllocator) findPartyByHint(ctx context.Context, hint string) (string, error) {
	partyIDPrefix := hint + "::"
	body, err := doRequest(ctx, p.httpClient, p.tokens, p.baseURL, httpRequest{
		method:    http.MethodGet,
		path:      allocatePath + "?filter-party=" + url.QueryEscape(partyIDPrefix),
		errPrefix: "party",
	})
	if err != nil {
		return "", err
	}
	var decoded listKnownPartiesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("party: decode known parties response: %w", err)
	}
	for _, details := range decoded.PartyDetails {
		if strings.HasPrefix(details.Party, partyIDPrefix) {
			return details.Party, nil
		}
	}
	return "", nil
}

func decodeAllocatedParty(body []byte) (string, error) {
	var decoded partyAllocateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("party: decode response: %w", err)
	}
	if strings.TrimSpace(decoded.PartyDetails.Party) == "" {
		return "", errors.New("party: response missing partyDetails.party")
	}
	return decoded.PartyDetails.Party, nil
}

func isPartyAlreadyExists(e *httpStatusError) bool {
	return e.statusCode == http.StatusBadRequest && strings.Contains(string(e.body), partyAlreadyExistsMarker)
}

func isRequestTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func randomSuffix(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
