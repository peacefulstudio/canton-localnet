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
	"io"
	"net/http"
	"strings"
)

// PartyAllocator allocates parties via POST /v2/parties.
//
// Each allocator instance generates a random hex suffix at construction
// time; allocate requests send a hint of the form "<consumer-prefix>-<suffix>"
// so that two test runs against the same ledger never collide on a party id.
// The suffix is stable across calls on the same instance, which keeps logs
// readable when several parties share a test.
type PartyAllocator struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
	suffix     string
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
		baseURL:    strings.TrimRight(baseURL, "/"),
		tokens:     tokens,
		httpClient: http.DefaultClient,
		suffix:     suffix,
	}
	for _, opt := range opts {
		opt(p)
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
	DisplayName        string `json:"displayName,omitempty"`
	IdentityProviderID string `json:"identityProviderId"`
}

type partyAllocateResponse struct {
	PartyDetails struct {
		Party string `json:"party"`
	} `json:"partyDetails"`
}

// Allocate posts a party-allocation request with hint "<prefix>-<suffix>"
// and returns the allocated party id from partyDetails.party. If
// displayName is empty the field is omitted from the payload.
func (p *PartyAllocator) Allocate(ctx context.Context, prefix, displayName string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(prefix) == "" {
		return "", errors.New("party: prefix is required")
	}
	payload := partyAllocateRequest{
		PartyIDHint:        p.Hint(prefix),
		DisplayName:        displayName,
		IdentityProviderID: "",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("party: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v2/parties", strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("party: build POST /v2/parties: %w", err)
	}
	token, err := p.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("party: acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.ContentLength = int64(len(raw))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("party: POST /v2/parties: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("party: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("party: POST /v2/parties returned HTTP %d: %s", resp.StatusCode, truncateBodyForError(body))
	}
	var decoded partyAllocateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("party: decode response: %w", err)
	}
	if strings.TrimSpace(decoded.PartyDetails.Party) == "" {
		return "", errors.New("party: response missing partyDetails.party")
	}
	return decoded.PartyDetails.Party, nil
}

func randomSuffix(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
