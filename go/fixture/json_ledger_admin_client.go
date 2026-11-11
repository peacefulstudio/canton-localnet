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
		httpClient: newDefaultHTTPClient(),
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

// GlobalSynchronizerAlias is the stable alias of the global synchronizer.
const GlobalSynchronizerAlias = "global"

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
	return c.listConnectedSynchronizers(ctx, "/v2/state/connected-synchronizers?party="+url.QueryEscape(party))
}

func (c *JsonLedgerAdminClient) listConnectedSynchronizers(ctx context.Context, path string) ([]ConnectedSynchronizer, error) {
	var out connectedSynchronizersResponse
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

// GetGlobalSynchronizerId returns the id of the connected synchronizer whose
// alias is GlobalSynchronizerAlias, from the participant-wide listing. Every
// LocalNet profile connects to it, single-sync or multi-sync, so it is the
// default target for party allocation. Errors if absent.
func (c *JsonLedgerAdminClient) GetGlobalSynchronizerId(ctx context.Context) (string, error) {
	syncs, err := c.listConnectedSynchronizers(ctx, "/v2/state/connected-synchronizers")
	if err != nil {
		return "", err
	}
	aliases := make([]string, 0, len(syncs))
	for _, s := range syncs {
		if s.Alias == GlobalSynchronizerAlias {
			return s.ID, nil
		}
		aliases = append(aliases, s.Alias)
	}
	return "", fmt.Errorf("admin: no connected synchronizer with alias %q; connected: %v", GlobalSynchronizerAlias, aliases)
}

type vettingListRequest struct {
	PackageMetadataFilter packageMetadataFilter `json:"packageMetadataFilter"`
	TopologyStateFilter   topologyStateFilter   `json:"topologyStateFilter"`
	PageToken             string                `json:"pageToken,omitempty"`
}

type packageMetadataFilter struct {
	PackageIDs []string `json:"packageIds"`
}

type topologyStateFilter struct {
	ParticipantIDs  []string `json:"participantIds"`
	SynchronizerIDs []string `json:"synchronizerIds,omitempty"`
}

type vettingListResponse struct {
	VettedPackages []struct {
		Packages []struct {
			PackageID string `json:"packageId"`
		} `json:"packages"`
		ParticipantID string `json:"participantId"`
	} `json:"vettedPackages"`
	NextPageToken string `json:"nextPageToken"`
}

func (c *JsonLedgerAdminClient) isPackageVetted(ctx context.Context, packageID, synchronizerID string) (bool, error) {
	if strings.TrimSpace(packageID) == "" {
		return false, errors.New("admin: packageID is required")
	}
	participantID, err := c.GetParticipantId(ctx)
	if err != nil {
		return false, err
	}
	request := vettingListRequest{
		PackageMetadataFilter: packageMetadataFilter{PackageIDs: []string{packageID}},
		TopologyStateFilter:   topologyStateFilter{ParticipantIDs: []string{participantID}},
	}
	if synchronizerID != "" {
		request.TopologyStateFilter.SynchronizerIDs = []string{synchronizerID}
	}
	for {
		payload, err := json.Marshal(request)
		if err != nil {
			return false, fmt.Errorf("admin: marshal vetting list request: %w", err)
		}
		var page vettingListResponse
		if err := c.doJSON(ctx, http.MethodPost, "/v2/package-vetting/list", payload, &page); err != nil {
			return false, err
		}
		if page.vets(participantID, packageID) {
			return true, nil
		}
		if page.NextPageToken == "" {
			return false, nil
		}
		request.PageToken = page.NextPageToken
	}
}

func (r vettingListResponse) vets(participantID, packageID string) bool {
	for _, entry := range r.VettedPackages {
		if entry.ParticipantID != participantID {
			continue
		}
		for _, p := range entry.Packages {
			if p.PackageID == packageID {
				return true
			}
		}
	}
	return false
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
