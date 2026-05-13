// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		baseURL:    normalizeBaseURL(baseURL),
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

func (c *JsonLedgerAdminClient) doJSON(ctx context.Context, method, path string, body []byte, out any) error {
	respBody, err := doRequest(ctx, c.httpClient, c.tokens, c.baseURL, httpRequest{
		method:      method,
		path:        path,
		body:        body,
		contentType: "application/json",
		errPrefix:   "admin",
	})
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("admin: decode %s %s response: %w", method, path, err)
	}
	return nil
}
