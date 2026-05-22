// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HS256TTL is the lifetime of a self-signed sv-validator-1 JWT minted by
// MintToken.
const HS256TTL = time.Hour

// MintToken returns a bearer token for ep that is valid against the
// slot's participant. For OAuth2 slots it performs a client_credentials
// exchange against ep.TokenURLHost. For HS256 slots it mints a
// self-signed JWT locally.
func MintToken(ctx context.Context, ep Endpoints, client *http.Client) (string, error) {
	switch ep.Slot.AuthKind {
	case AuthKindOAuth2:
		return mintOAuth2(ctx, ep, client)
	case AuthKindHS256:
		return mintHS256(ep, time.Now)
	default:
		return "", fmt.Errorf("slot: unknown auth kind %q", ep.Slot.AuthKind)
	}
}

func mintOAuth2(ctx context.Context, ep Endpoints, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ep.TokenURLHost == "" {
		return "", errors.New("slot: oauth2 token URL not resolved")
	}
	if ep.ClientSecret == "" {
		return "", fmt.Errorf("slot: %s: client secret not found — set CANTON_LOCALNET_%s_CLIENT_SECRET, pass --repo-root so compose/modules/keycloak/env/%s/on/oauth2.env is readable, or boot Keycloak with the 'on' profile so that file exists", ep.Slot.Canonical, ep.Slot.EnvPrefix(), ep.Slot.Canonical)
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", ep.ClientID)
	form.Set("client_secret", ep.ClientSecret)
	if ep.Audience != "" {
		form.Set("audience", ep.Audience)
	}
	if ep.Scope != "" {
		form.Set("scope", ep.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.TokenURLHost, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("slot: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slot: token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return "", fmt.Errorf("slot: read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("slot: token endpoint %s returned HTTP %d: %s", ep.TokenURLHost, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("slot: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("slot: token endpoint returned empty access_token")
	}
	return tr.AccessToken, nil
}

func mintHS256(ep Endpoints, now func() time.Time) (string, error) {
	iat := now().Unix()
	exp := now().Add(HS256TTL).Unix()
	payload := map[string]any{
		"sub": ep.HS256User,
		"aud": ep.Audience,
		"iat": iat,
		"exp": exp,
	}
	return signHS256(ep.HS256Secret, payload)
}

func signHS256(secret string, payload map[string]any) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encode := func(b []byte) string {
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := encode(headerJSON) + "." + encode(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := mac.Sum(nil)
	return signingInput + "." + encode(signature), nil
}
