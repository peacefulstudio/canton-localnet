// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TokenProvider abstracts access-token retrieval for the admin client.
// OAuth2TokenProvider implements this interface; tests can substitute a
// static or scripted provider.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// JsonLedgerAdminClient is a minimal HTTP client for the Canton JSON Ledger
// Admin API.
type JsonLedgerAdminClient struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
}

// JsonLedgerAdminClientOption mutates a JsonLedgerAdminClient at construction.
type JsonLedgerAdminClientOption func(*JsonLedgerAdminClient)

// WithHTTPClient overrides the default HTTP client used by the admin client.
func WithHTTPClient(c *http.Client) JsonLedgerAdminClientOption {
	return func(j *JsonLedgerAdminClient) {
		if c != nil {
			j.httpClient = c
		}
	}
}

// NewJsonLedgerAdminClient constructs a JsonLedgerAdminClient.
//
// baseURL is the JSON Ledger API origin, e.g. "http://localhost:3975".
// tokens supplies bearer tokens for the Authorization header.
func NewJsonLedgerAdminClient(baseURL string, tokens TokenProvider, opts ...JsonLedgerAdminClientOption) (*JsonLedgerAdminClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("admin: baseURL is required")
	}
	if tokens == nil {
		return nil, errors.New("admin: TokenProvider is required")
	}
	client := &JsonLedgerAdminClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		tokens:     tokens,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

type participantIDResponse struct {
	ParticipantID string `json:"participantId"`
}

// GetParticipantId calls GET /v2/parties/participant-id and returns the
// participantId field from the JSON response body.
func (c *JsonLedgerAdminClient) GetParticipantId(ctx context.Context) (string, error) {
	var out participantIDResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/parties/participant-id", nil, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.ParticipantID) == "" {
		return "", errors.New("admin: response missing participantId")
	}
	return out.ParticipantID, nil
}

func (c *JsonLedgerAdminClient) doJSON(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("admin: build %s %s: %w", method, path, err)
	}
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("admin: acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("admin: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("admin: read %s %s response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("admin: %s %s returned HTTP %d: %s", method, path, resp.StatusCode, truncateBodyForError(raw))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("admin: decode %s %s response: %w", method, path, err)
	}
	return nil
}
