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

// UserOptions describes the user to create and the rights to grant.
type UserOptions struct {
	// UserID is the ledger user id (POST body field "user.id"). Required.
	UserID string
	// PrimaryParty is the user's primary party (POST body field
	// "user.primaryParty"). Required.
	PrimaryParty string
	// ActAs lists parties for which the user receives CanActAs rights.
	ActAs []string
	// ReadAs lists parties for which the user receives CanReadAs rights.
	ReadAs []string
	// Annotations is an optional map written into user.metadata.annotations.
	Annotations map[string]string
}

// UserBuilder creates ledger users (POST /v2/users) and grants act-as /
// read-as rights (POST /v2/users/{id}/rights).
type UserBuilder struct {
	baseURL    string
	tokens     TokenProvider
	httpClient *http.Client
}

// UserBuilderOption mutates a UserBuilder at construction.
type UserBuilderOption func(*UserBuilder)

// WithUserBuilderHTTPClient overrides the default HTTP client.
func WithUserBuilderHTTPClient(c *http.Client) UserBuilderOption {
	return func(b *UserBuilder) {
		if c != nil {
			b.httpClient = c
		}
	}
}

// NewUserBuilder constructs a UserBuilder.
func NewUserBuilder(baseURL string, tokens TokenProvider, opts ...UserBuilderOption) (*UserBuilder, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("user: baseURL is required")
	}
	if tokens == nil {
		return nil, errors.New("user: TokenProvider is required")
	}
	b := &UserBuilder{
		baseURL:    strings.TrimRight(baseURL, "/"),
		tokens:     tokens,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

type createUserRequest struct {
	User createUserPayload `json:"user"`
	// Rights here is always the empty array; rights are granted via the
	// rights sub-resource below.
	Rights []any `json:"rights"`
}

type createUserPayload struct {
	ID                 string       `json:"id"`
	IsDeactivated      bool         `json:"isDeactivated"`
	PrimaryParty       string       `json:"primaryParty"`
	IdentityProviderID string       `json:"identityProviderId"`
	Metadata           userMetadata `json:"metadata"`
}

type userMetadata struct {
	ResourceVersion string            `json:"resourceVersion"`
	Annotations     map[string]string `json:"annotations"`
}

type createUserResponse struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
}

type grantRightsRequest struct {
	UserID             string  `json:"userId"`
	IdentityProviderID string  `json:"identityProviderId"`
	Rights             []right `json:"rights"`
}

type right struct {
	Kind rightKind `json:"kind"`
}

type rightKind struct {
	CanActAs  *partyRight `json:"CanActAs,omitempty"`
	CanReadAs *partyRight `json:"CanReadAs,omitempty"`
}

type partyRight struct {
	Value partyRef `json:"value"`
}

type partyRef struct {
	Party string `json:"party"`
}

// Create creates the user and, if any ActAs/ReadAs entries are supplied,
// grants the corresponding rights. The returned id is the user id echoed by
// the server. Create returns an error on the first failed step; partial
// state (user created but rights grant failed) is not rolled back.
func (b *UserBuilder) Create(ctx context.Context, opts UserOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return "", errors.New("user: UserID is required")
	}
	if strings.TrimSpace(opts.PrimaryParty) == "" {
		return "", errors.New("user: PrimaryParty is required")
	}

	annotations := opts.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	createBody := createUserRequest{
		User: createUserPayload{
			ID:                 opts.UserID,
			IsDeactivated:      false,
			PrimaryParty:       opts.PrimaryParty,
			IdentityProviderID: "",
			Metadata: userMetadata{
				ResourceVersion: "",
				Annotations:     annotations,
			},
		},
		Rights: []any{},
	}
	var createResp createUserResponse
	if err := b.postJSON(ctx, "/v2/users", createBody, &createResp); err != nil {
		return "", err
	}
	id := createResp.User.ID
	if strings.TrimSpace(id) == "" {
		id = opts.UserID
	}

	if len(opts.ActAs) == 0 && len(opts.ReadAs) == 0 {
		return id, nil
	}
	grant := grantRightsRequest{
		UserID:             opts.UserID,
		IdentityProviderID: "",
		Rights:             buildRights(opts.ActAs, opts.ReadAs),
	}
	if err := b.postJSON(ctx, "/v2/users/"+opts.UserID+"/rights", grant, nil); err != nil {
		return "", err
	}
	return id, nil
}

func buildRights(actAs, readAs []string) []right {
	rights := make([]right, 0, len(actAs)+len(readAs))
	for _, party := range actAs {
		rights = append(rights, right{Kind: rightKind{CanActAs: &partyRight{Value: partyRef{Party: party}}}})
	}
	for _, party := range readAs {
		rights = append(rights, right{Kind: rightKind{CanReadAs: &partyRight{Value: partyRef{Party: party}}}})
	}
	return rights
}

func (b *UserBuilder) postJSON(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("user: marshal %s body: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+path, strings.NewReader(string(raw)))
	if err != nil {
		return fmt.Errorf("user: build POST %s: %w", path, err)
	}
	token, err := b.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("user: acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.ContentLength = int64(len(raw))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("user: POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("user: read POST %s response: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("user: POST %s returned HTTP %d: %s", path, resp.StatusCode, truncateBodyForError(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("user: decode POST %s response: %w", path, err)
	}
	return nil
}
