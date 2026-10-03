// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package wallet drives a slot's Splice validator wallet API: it mints
// the wallet admin user's bearer token and taps Amulet into the
// validator party's wallet.
package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	tapPath       = "/api/validator/v0/wallet/tap"
	responseLimit = 1 << 20
	maxScale      = 10
)

var positiveDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// Credentials is the wallet admin user's resource-owner login against a
// slot's Keycloak realm. The wallet API authenticates the human wallet
// user, not the validator's service-account client.
type Credentials struct {
	TokenURL string
	ClientID string
	Username string
	Password string
}

// ValidateAmount rejects anything the wallet would not mint from: not a
// plain decimal, zero, or more than ten fractional digits. The amount is
// kept as text so no precision is lost on its way to the wallet.
func ValidateAmount(amount string) error {
	if !positiveDecimal.MatchString(amount) {
		return fmt.Errorf("wallet: amount %q must be a positive decimal such as 10 or 2.5", amount)
	}
	whole, fraction, _ := strings.Cut(amount, ".")
	if len(fraction) > maxScale {
		return fmt.Errorf("wallet: amount %q has more than %d fractional digits", amount, maxScale)
	}
	if strings.Trim(whole+fraction, "0") == "" {
		return fmt.Errorf("wallet: amount %q must be greater than zero", amount)
	}
	return nil
}

// MintToken logs the wallet admin user in with the password grant and
// returns the access token. Transient answers are retried within retry's
// budget.
func MintToken(ctx context.Context, creds Credentials, client *http.Client, retry Retry) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", creds.ClientID)
	form.Set("username", creds.Username)
	form.Set("password", creds.Password)
	form.Set("scope", "openid")

	got, err := retry.run(ctx, transientAnswer, func() (answer, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, creds.TokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return answer{}, fmt.Errorf("wallet: build token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return answer{}, fmt.Errorf("wallet: token request: %w", err)
		}
		defer resp.Body.Close()
		got, err := readAnswer(resp)
		if err != nil {
			return got, fmt.Errorf("wallet: read token response: %w", err)
		}
		return got, nil
	})
	if err != nil {
		return "", err
	}
	if got.status < 200 || got.status >= 300 {
		return "", fmt.Errorf("wallet: token endpoint %s returned HTTP %d for wallet user %s", creds.TokenURL, got.status, creds.Username)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(got.body, &tr); err != nil {
		return "", fmt.Errorf("wallet: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("wallet: token endpoint returned empty access_token")
	}
	return tr.AccessToken, nil
}

// Client calls a validator app's wallet API. Token is the wallet admin
// user's bearer; it is never rendered into output or into an error.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Retry   Retry
}

// Tap mints amount of Amulet into the validator party's wallet and
// returns the contract id of the new Amulet. Every call mints a new
// contract, so repeating it adds to the balance. It fails on any
// non-2xx response, after retrying the answers that guarantee the request was not processed
// (429, connection refused) within Retry's budget and naming the last status.
// A 502, 503 or 504 is not retried: the mint may have committed behind it.
func (c Client) Tap(ctx context.Context, amount string) (string, error) {
	if err := ValidateAmount(amount); err != nil {
		return "", err
	}
	request, err := json.Marshal(struct {
		Amount string `json:"amount"`
	}{Amount: amount})
	if err != nil {
		return "", fmt.Errorf("wallet: encode tap request: %w", err)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + tapPath
	got, err := c.Retry.run(ctx, requestNeverProcessed, func() (answer, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(request))
		if err != nil {
			return answer{}, fmt.Errorf("wallet: build tap request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return answer{}, fmt.Errorf("wallet: POST %s: %w", endpoint, err)
		}
		defer resp.Body.Close()
		got, err := readAnswer(resp)
		if err != nil {
			return got, fmt.Errorf("wallet: read tap response: %w", err)
		}
		return got, nil
	})
	if err != nil {
		return "", err
	}
	if got.status < 200 || got.status >= 300 {
		return "", fmt.Errorf("wallet: POST %s returned HTTP %d: %s", endpoint, got.status, strings.TrimSpace(string(got.body)))
	}
	var payload struct {
		ContractID string `json:"contract_id"`
	}
	if err := json.Unmarshal(got.body, &payload); err != nil {
		return "", fmt.Errorf("wallet: the tap succeeded but its response could not be read: %w", err)
	}
	if payload.ContractID == "" {
		return "", errors.New("wallet: the tap response carried no contract_id")
	}
	return payload.ContractID, nil
}
