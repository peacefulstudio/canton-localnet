// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package rights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const responseLimit = 8 << 20

// Client reads and revokes a ledger user's rights over a participant's
// JSON Ledger API. Token is a participant-admin bearer; it is never
// rendered into output or into an error.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// PrimaryParty returns the party the participant records as userID's
// own. It is the party a prune preserves: reading it from the
// participant is what keeps the preserved party right for slots whose
// validator party does not follow their slot name. It comes back empty
// for a user that has none.
func (c Client) PrimaryParty(ctx context.Context, userID string) (string, error) {
	body, err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(userID), nil)
	if err != nil {
		return "", err
	}
	var payload struct {
		User struct {
			PrimaryParty string `json:"primaryParty"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("rights: decode user %s: %w", userID, err)
	}
	return payload.User.PrimaryParty, nil
}

// List returns the rights the participant currently holds for userID.
func (c Client) List(ctx context.Context, userID string) ([]Right, error) {
	body, err := c.do(ctx, http.MethodGet, c.rightsPath(userID), nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Rights []Right `json:"rights"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("rights: decode rights of user %s: %w", userID, err)
	}
	return payload.Rights, nil
}

// Revoke drops the given rights from userID and returns the rights the
// participant reported newly revoked. It fails when the participant's
// response does not confirm every requested right was revoked, because
// an unconfirmed right is still held and a caller that ignored this
// would report a clean sweep that did not happen.
func (c Client) Revoke(ctx context.Context, userID string, revoke []Right) ([]Right, error) {
	request, err := json.Marshal(struct {
		UserID string  `json:"userId"`
		Rights []Right `json:"rights"`
	}{UserID: userID, Rights: revoke})
	if err != nil {
		return nil, fmt.Errorf("rights: encode revoke request: %w", err)
	}
	body, err := c.do(ctx, http.MethodPatch, c.rightsPath(userID), request)
	if err != nil {
		return nil, err
	}
	var payload struct {
		NewlyRevokedRights []Right `json:"newlyRevokedRights"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("rights: the revoke request succeeded but its response could not be read, so the rights may already be gone: %w", err)
	}
	confirmed := make(map[rightIdentity]bool, len(payload.NewlyRevokedRights))
	for _, right := range payload.NewlyRevokedRights {
		confirmed[right.identity()] = true
	}
	var confirmedCount int
	for _, right := range revoke {
		if confirmed[right.identity()] {
			confirmedCount++
		}
	}
	if confirmedCount != len(revoke) {
		return nil, fmt.Errorf("rights: the participant revoked %d of %d right(s) — the rest are still held; re-run to see what remains", confirmedCount, len(revoke))
	}
	return payload.NewlyRevokedRights, nil
}

func (c Client) rightsPath(userID string) string {
	return "/v2/users/" + url.PathEscape(userID) + "/rights"
}

func (c Client) do(ctx context.Context, method, path string, request []byte) ([]byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	var payload io.Reader
	if request != nil {
		payload = bytes.NewReader(request)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("rights: build %s %s: %w", method, endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rights: %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return nil, fmt.Errorf("rights: read %s %s response: %w", method, endpoint, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rights: %s %s returned HTTP %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
