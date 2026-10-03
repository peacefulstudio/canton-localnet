// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	balancePath      = "/api/validator/v0/wallet/balance"
	miningRoundsPath = "/api/validator/v0/scan-proxy/open-and-issuing-mining-rounds"
	maxFundingTaps   = 3
	fractionalDigits = 10
)

// Balance returns the validator wallet's spendable Amulet quantity as the
// wallet reports it (effective_unlocked_qty): the unlocked holdings net of
// holding fees. Locked Amulet is excluded because it cannot be spent.
func (c Client) Balance(ctx context.Context) (string, error) {
	body, err := c.get(ctx, balancePath)
	if err != nil {
		return "", err
	}
	var payload struct {
		EffectiveUnlockedQty string `json:"effective_unlocked_qty"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("wallet: decode balance response: %w", err)
	}
	if _, err := parseDecimal(payload.EffectiveUnlockedQty); err != nil {
		return "", fmt.Errorf("wallet: balance response effective_unlocked_qty: %w", err)
	}
	return payload.EffectiveUnlockedQty, nil
}

// AmuletPrice returns the USD price of one Amulet in the latest open
// mining round that has already opened at now, the round a tap is minted
// in. A zero now means the current time.
func (c Client) AmuletPrice(ctx context.Context, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	body, err := c.get(ctx, miningRoundsPath)
	if err != nil {
		return "", err
	}
	var payload struct {
		OpenMiningRounds []struct {
			Contract struct {
				Payload struct {
					AmuletPrice string `json:"amuletPrice"`
					OpensAt     string `json:"opensAt"`
				} `json:"payload"`
			} `json:"contract"`
		} `json:"open_mining_rounds"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("wallet: decode mining rounds response: %w", err)
	}
	var latestOpen time.Time
	price := ""
	for _, round := range payload.OpenMiningRounds {
		opensAt, err := time.Parse(time.RFC3339Nano, round.Contract.Payload.OpensAt)
		if err != nil {
			return "", fmt.Errorf("wallet: mining round opensAt %q: %w", round.Contract.Payload.OpensAt, err)
		}
		if opensAt.After(now) || (price != "" && !opensAt.After(latestOpen)) {
			continue
		}
		latestOpen, price = opensAt, round.Contract.Payload.AmuletPrice
	}
	if price == "" {
		return "", errors.New("wallet: no open mining round has opened yet, so there is no amulet price")
	}
	if parsed, err := parseDecimal(price); err != nil || parsed.Sign() <= 0 {
		return "", fmt.Errorf("wallet: mining round amuletPrice %q must be a positive decimal", price)
	}
	return price, nil
}

// TapAtLeast raises the validator wallet's spendable balance to target
// Amulet and returns the balance it last read. A balance already at or
// above target is left alone with no tap. Otherwise it taps the gap,
// converted to USD at the current amulet price and rounded up so the
// mint cannot land short, then re-reads the balance. Holding fees or a
// price tick between the read and the mint can leave it short, so it
// repeats up to three taps before failing. Now is the clock used to pick
// the open mining round; nil means time.Now.
func (c Client) TapAtLeast(ctx context.Context, target string, now func() time.Time) (string, error) {
	if err := ValidateAmount(target); err != nil {
		return "", err
	}
	targetAmulet, _ := parseDecimal(target)
	if now == nil {
		now = time.Now
	}
	balance, err := c.Balance(ctx)
	if err != nil {
		return "", err
	}
	for taps := 0; ; taps++ {
		held, _ := parseDecimal(balance)
		if held.Cmp(targetAmulet) >= 0 {
			return balance, nil
		}
		if taps == maxFundingTaps {
			return balance, fmt.Errorf("wallet: balance %s is still below %s Amulet after %d taps", balance, target, maxFundingTaps)
		}
		price, err := c.AmuletPrice(ctx, now())
		if err != nil {
			return "", err
		}
		priceRat, _ := parseDecimal(price)
		usd := roundUpUSD(new(big.Rat).Mul(new(big.Rat).Sub(targetAmulet, held), priceRat))
		if _, err := c.Tap(ctx, usd); err != nil {
			return "", err
		}
		if balance, err = c.Balance(ctx); err != nil {
			return "", err
		}
	}
}

func (c Client) get(ctx context.Context, path string) ([]byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	got, err := c.Retry.run(ctx, requestNeverProcessed, func() (answer, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return answer{}, fmt.Errorf("wallet: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return answer{}, fmt.Errorf("wallet: GET %s: %w", endpoint, err)
		}
		defer resp.Body.Close()
		got, err := readAnswer(resp)
		if err != nil {
			return got, fmt.Errorf("wallet: read response of GET %s: %w", endpoint, err)
		}
		return got, nil
	})
	if err != nil {
		return nil, err
	}
	if got.status < 200 || got.status >= 300 {
		return nil, fmt.Errorf("wallet: GET %s returned HTTP %d: %s", endpoint, got.status, strings.TrimSpace(string(got.body)))
	}
	return got.body, nil
}

func parseDecimal(text string) (*big.Rat, error) {
	if !positiveDecimal.MatchString(text) {
		return nil, fmt.Errorf("%q is not a non-negative decimal", text)
	}
	value, _ := new(big.Rat).SetString(text)
	return value, nil
}

func roundUpUSD(usd *big.Rat) string {
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(fractionalDigits), nil)
	scaled := new(big.Rat).Mul(usd, new(big.Rat).SetInt(divisor))
	units, remainder := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	if remainder.Sign() > 0 {
		units.Add(units, big.NewInt(1))
	}
	whole, fraction := new(big.Int).QuoRem(units, divisor, new(big.Int))
	padded := strings.Repeat("0", fractionalDigits) + fraction.String()
	text := whole.String() + "." + padded[len(padded)-fractionalDigits:]
	return strings.TrimRight(strings.TrimRight(text, "0"), ".")
}
