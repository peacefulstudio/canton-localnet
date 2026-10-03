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
	"net/url"
	"strings"
)

// FetchPrimaryParty calls GET /v2/users/{ep.ValidatorUserID} on the slot's
// JSON Ledger API using the supplied bearer token and returns the user's
// primaryParty. It fails when the user has no primary party set.
func FetchPrimaryParty(ctx context.Context, ep Endpoints, token string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(ep.ValidatorUserID) == "" {
		return "", fmt.Errorf("slot: %s: validator user id not resolved", ep.Slot.Canonical)
	}
	endpoint := strings.TrimRight(ep.JSONLedgerAPIURL, "/") + "/v2/users/" + url.PathEscape(ep.ValidatorUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("slot: build user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slot: user request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return "", fmt.Errorf("slot: read user response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("slot: user endpoint %s returned HTTP %d", endpoint, resp.StatusCode)
	}
	var out struct {
		User struct {
			PrimaryParty string `json:"primaryParty"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("slot: decode user response: %w", err)
	}
	if strings.TrimSpace(out.User.PrimaryParty) == "" {
		return "", errors.New("slot: user response has no primaryParty")
	}
	return out.User.PrimaryParty, nil
}
