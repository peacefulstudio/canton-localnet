// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
// baseURL is the JSON Ledger API origin, e.g. "http://localhost:11975".
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

// AppSynchronizerAlias is the stable alias of the app-provider synchronizer.
const AppSynchronizerAlias = "app-synchronizer"

// ConnectedSynchronizer is one entry from GET /v2/state/connected-synchronizers.
type ConnectedSynchronizer struct {
	Alias      string
	ID         string
	Permission string
}

type connectedSynchronizersResponse struct {
	ConnectedSynchronizers []struct {
		SynchronizerAlias string `json:"synchronizerAlias"`
		SynchronizerID    string `json:"synchronizerId"`
		Permission        string `json:"permission"`
	} `json:"connectedSynchronizers"`
}

// GetConnectedSynchronizers lists the synchronizers the participant is
// connected to for party, via GET /v2/state/connected-synchronizers.
func (c *JsonLedgerAdminClient) GetConnectedSynchronizers(ctx context.Context, party string) ([]ConnectedSynchronizer, error) {
	if strings.TrimSpace(party) == "" {
		return nil, errors.New("admin: party is required")
	}
	var out connectedSynchronizersResponse
	path := "/v2/state/connected-synchronizers?party=" + url.QueryEscape(party)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	result := make([]ConnectedSynchronizer, 0, len(out.ConnectedSynchronizers))
	for _, s := range out.ConnectedSynchronizers {
		if strings.TrimSpace(s.SynchronizerID) == "" {
			return nil, fmt.Errorf("admin: connected-synchronizers entry missing synchronizerId (alias=%q)", s.SynchronizerAlias)
		}
		result = append(result, ConnectedSynchronizer{Alias: s.SynchronizerAlias, ID: s.SynchronizerID, Permission: s.Permission})
	}
	return result, nil
}

// GetAppSynchronizerId returns the id of the connected synchronizer whose
// alias is AppSynchronizerAlias. Errors if absent (multi-sync off).
func (c *JsonLedgerAdminClient) GetAppSynchronizerId(ctx context.Context, party string) (string, error) {
	syncs, err := c.GetConnectedSynchronizers(ctx, party)
	if err != nil {
		return "", err
	}
	aliases := make([]string, 0, len(syncs))
	for _, s := range syncs {
		if s.Alias == AppSynchronizerAlias {
			return s.ID, nil
		}
		aliases = append(aliases, s.Alias)
	}
	return "", fmt.Errorf("admin: no connected synchronizer with alias %q (is multi-sync enabled?); connected: %v", AppSynchronizerAlias, aliases)
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
