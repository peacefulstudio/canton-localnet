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
	"net/url"
	"strings"
	"sync"
	"time"
)

// RefreshSkew is the safety margin subtracted from the access token's nominal
// expiry. A token is refreshed once its remaining lifetime drops below this
// threshold, so callers never present an about-to-expire token to the API.
const RefreshSkew = 30 * time.Second

// OAuth2TokenProviderConfig configures an OAuth2TokenProvider.
type OAuth2TokenProviderConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Audience     string
	Scope        string
	HTTPClient   *http.Client
	Now          func() time.Time
}

// OAuth2TokenProvider obtains and caches OAuth2 access tokens via the
// client_credentials grant. It is safe for concurrent use; in-flight refreshes
// are coalesced so concurrent callers don't trigger redundant token requests.
type OAuth2TokenProvider struct {
	cfg OAuth2TokenProviderConfig

	mu          sync.Mutex
	cached      string
	expiresAt   time.Time
	refreshing  bool
	refreshDone chan struct{}
	refreshErr  error
}

// NewOAuth2TokenProvider constructs an OAuth2TokenProvider.
//
// TokenURL, ClientID and ClientSecret are required. If HTTPClient is nil,
// http.DefaultClient is used. If Now is nil, time.Now is used.
func NewOAuth2TokenProvider(cfg OAuth2TokenProviderConfig) (*OAuth2TokenProvider, error) {
	if strings.TrimSpace(cfg.TokenURL) == "" {
		return nil, errors.New("oauth2: TokenURL is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("oauth2: ClientID is required")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("oauth2: ClientSecret is required")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &OAuth2TokenProvider{cfg: cfg}, nil
}

// Token returns a cached access token, refreshing it via the token endpoint
// if the current one is missing or within RefreshSkew of expiry.
func (p *OAuth2TokenProvider) Token(ctx context.Context) (string, error) {
	for {
		p.mu.Lock()
		if p.tokenIsFreshLocked() {
			token := p.cached
			p.mu.Unlock()
			return token, nil
		}
		if p.refreshing {
			done := p.refreshDone
			p.mu.Unlock()
			select {
			case <-done:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			p.mu.Lock()
			leaderErr := p.refreshErr
			cached := p.cached
			p.mu.Unlock()

			if leaderErr == nil {
				return cached, nil
			}
			if isContextError(leaderErr) && ctx.Err() == nil {
				continue
			}
			return "", leaderErr
		}
		p.refreshing = true
		p.refreshDone = make(chan struct{})
		p.refreshErr = nil
		p.mu.Unlock()

		token, expiresAt, err := p.fetchToken(ctx)

		p.mu.Lock()
		if err != nil {
			p.refreshErr = err
		} else {
			p.cached = token
			p.expiresAt = expiresAt
		}
		close(p.refreshDone)
		p.refreshing = false
		p.mu.Unlock()
		if err != nil {
			return "", err
		}
		return token, nil
	}
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

const errorBodyMaxBytes = 512

func truncateBodyForError(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) <= errorBodyMaxBytes {
		return trimmed
	}
	return trimmed[:errorBodyMaxBytes] + "...[truncated]"
}

// Invalidate drops the cached token. The next call to Token forces a refresh.
func (p *OAuth2TokenProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cached = ""
	p.expiresAt = time.Time{}
}

func (p *OAuth2TokenProvider) tokenIsFreshLocked() bool {
	if p.cached == "" {
		return false
	}
	return p.cfg.Now().Add(RefreshSkew).Before(p.expiresAt)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (p *OAuth2TokenProvider) fetchToken(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)
	if p.cfg.Audience != "" {
		form.Set("audience", p.cfg.Audience)
	}
	if p.cfg.Scope != "" {
		form.Set("scope", p.cfg.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: read token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er tokenErrorResponse
		_ = json.Unmarshal(body, &er)
		if er.Error != "" {
			if er.ErrorDescription != "" {
				return "", time.Time{}, fmt.Errorf("oauth2: token endpoint returned HTTP %d: %s: %s", resp.StatusCode, er.Error, er.ErrorDescription)
			}
			return "", time.Time{}, fmt.Errorf("oauth2: token endpoint returned HTTP %d: %s", resp.StatusCode, er.Error)
		}
		return "", time.Time{}, fmt.Errorf("oauth2: token endpoint returned HTTP %d: %s", resp.StatusCode, truncateBodyForError(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("oauth2: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, errors.New("oauth2: token endpoint returned empty access_token")
	}
	if tr.ExpiresIn <= 0 {
		return "", time.Time{}, fmt.Errorf("oauth2: token endpoint returned non-positive expires_in (%d)", tr.ExpiresIn)
	}
	return tr.AccessToken, p.cfg.Now().Add(time.Duration(tr.ExpiresIn) * time.Second), nil
}
