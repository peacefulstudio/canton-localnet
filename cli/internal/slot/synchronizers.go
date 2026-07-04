// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ConnectedSynchronizer is one entry returned by the JSON Ledger API
// GET /v2/state/connected-synchronizers endpoint.
type ConnectedSynchronizer struct {
	Alias string
	ID    string
}

// FetchConnectedSynchronizers calls GET /v2/state/connected-synchronizers
// and returns the participant's connected synchronizers.
func FetchConnectedSynchronizers(ctx context.Context, ep Endpoints, token string, client *http.Client) ([]ConnectedSynchronizer, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u := strings.TrimRight(ep.JSONLedgerAPIURL, "/") + "/v2/state/connected-synchronizers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("slot: build connected-synchronizers request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slot: connected-synchronizers request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return nil, fmt.Errorf("slot: read connected-synchronizers response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("slot: connected-synchronizers endpoint %s returned HTTP %d: %s", u, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		ConnectedSynchronizers []struct {
			SynchronizerAlias string `json:"synchronizerAlias"`
			SynchronizerID    string `json:"synchronizerId"`
		} `json:"connectedSynchronizers"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("slot: decode connected-synchronizers response: %w", err)
	}
	result := make([]ConnectedSynchronizer, 0, len(out.ConnectedSynchronizers))
	for _, s := range out.ConnectedSynchronizers {
		if strings.TrimSpace(s.SynchronizerID) == "" {
			return nil, errors.New("slot: connected-synchronizers entry missing synchronizerId")
		}
		result = append(result, ConnectedSynchronizer{Alias: s.SynchronizerAlias, ID: s.SynchronizerID})
	}
	return result, nil
}
