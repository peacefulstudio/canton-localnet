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

// FetchParticipantID calls GET /v2/parties/participant-id on the slot's
// JSON Ledger API using the supplied bearer token. The returned id has
// the form "<participant-hint>::<namespace>".
func FetchParticipantID(ctx context.Context, ep Endpoints, token string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url := strings.TrimRight(ep.JSONLedgerAPIURL, "/") + "/v2/parties/participant-id"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("slot: build participant-id request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slot: participant-id request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return "", fmt.Errorf("slot: read participant-id response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("slot: participant-id endpoint %s returned HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		ParticipantID string `json:"participantId"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("slot: decode participant-id response: %w", err)
	}
	if strings.TrimSpace(out.ParticipantID) == "" {
		return "", errors.New("slot: participant-id response missing participantId")
	}
	return out.ParticipantID, nil
}

// SplitParticipantID returns (hint, namespace) from a participant id of
// the form "<hint>::<namespace>". Errors if the separator is missing or
// either side is empty — both halves are mandatory in a well-formed
// Canton participant id.
func SplitParticipantID(id string) (string, string, error) {
	idx := strings.Index(id, "::")
	if idx < 0 {
		return "", "", fmt.Errorf("slot: participant id %q missing :: separator", id)
	}
	hint, namespace := id[:idx], id[idx+2:]
	if hint == "" || namespace == "" {
		return "", "", fmt.Errorf("slot: participant id %q has empty hint or namespace", id)
	}
	return hint, namespace, nil
}
