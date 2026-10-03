// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package wallet_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/peacefulstudio/canton-localnet/cli/internal/wallet"
)

const miningRounds = `{"open_mining_rounds":[
{"contract":{"payload":{"amuletPrice":"0.005","opensAt":"2026-10-03T11:29:26.885888Z"}}},
{"contract":{"payload":{"amuletPrice":"0.01","opensAt":"2026-10-03T11:39:26.885888Z"}}}],
"issuing_mining_rounds":[]}`

func fixedNow() time.Time { return time.Date(2026, 10, 3, 11, 35, 0, 0, time.UTC) }

type walletFake struct {
	mu          sync.Mutex
	balances    []string
	reads       int
	tapBodies   []string
	roundsCode  int
	balanceCode int
}

func (f *walletFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/validator/v0/wallet/balance":
		if f.balanceCode != 0 {
			w.WriteHeader(f.balanceCode)
			return
		}
		index := min(f.reads, len(f.balances)-1)
		f.reads++
		_, _ = fmt.Fprintf(w, `{"round":1,"effective_unlocked_qty":%q,"effective_locked_qty":"0.0000000000","total_holding_fees":"0.0000000000"}`, f.balances[index])
	case "/api/validator/v0/scan-proxy/open-and-issuing-mining-rounds":
		if f.roundsCode != 0 {
			w.WriteHeader(f.roundsCode)
			return
		}
		_, _ = w.Write([]byte(miningRounds))
	case "/api/validator/v0/wallet/tap":
		raw, _ := io.ReadAll(r.Body)
		f.tapBodies = append(f.tapBodies, string(raw))
		_, _ = w.Write([]byte(`{"contract_id":"00new"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func fundingClient(t *testing.T, fake *walletFake) wallet.Client {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return wallet.Client{BaseURL: server.URL, Token: "t"}
}

func TestTapAtLeast_is_a_no_op_when_the_balance_is_already_at_the_target(t *testing.T) {
	for _, held := range []string{"1000.0000000000", "1500.5"} {
		fake := &walletFake{balances: []string{held}}

		balance, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)
		if err != nil {
			t.Fatal(err)
		}

		if balance != held || len(fake.tapBodies) != 0 {
			t.Fatalf("held %s: balance = %s, taps = %v, want the balance untouched and no tap", held, balance, fake.tapBodies)
		}
	}
}

func TestTapAtLeast_taps_exactly_the_gap_converted_at_the_latest_opened_round_price(t *testing.T) {
	fake := &walletFake{balances: []string{"246.9135780200", "1000.0000000000"}}

	balance, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)
	if err != nil {
		t.Fatal(err)
	}

	if balance != "1000.0000000000" {
		t.Fatalf("balance = %s, want 1000.0000000000", balance)
	}
	if len(fake.tapBodies) != 1 || fake.tapBodies[0] != `{"amount":"3.7654321099"}` {
		t.Fatalf("taps = %v, want one tap with usd amount \"3.7654321099\" ((1000 - 246.91357802) * 0.005)", fake.tapBodies)
	}
}

func TestTapAtLeast_rounds_the_usd_amount_up_so_the_mint_cannot_land_short(t *testing.T) {
	fake := &walletFake{balances: []string{"0.0000000000", "1000"}}

	if _, err := fundingClient(t, fake).TapAtLeast(context.Background(), "0.0000000001", fixedNow); err != nil {
		t.Fatal(err)
	}

	if len(fake.tapBodies) != 1 || fake.tapBodies[0] != `{"amount":"0.0000000001"}` {
		t.Fatalf("taps = %v, want usd amount \"0.0000000001\" (5e-13 rounded up to one unit of scale 10)", fake.tapBodies)
	}
}

func TestTapAtLeast_taps_again_when_the_first_tap_undershoots(t *testing.T) {
	fake := &walletFake{balances: []string{"0", "990.0000000000", "1000.0000000000"}}

	balance, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{`{"amount":"5"}`, `{"amount":"0.05"}`}
	if balance != "1000.0000000000" || strings.Join(fake.tapBodies, ",") != strings.Join(want, ",") {
		t.Fatalf("balance = %s, taps = %v, want taps %v", balance, fake.tapBodies, want)
	}
}

func TestTapAtLeast_gives_up_after_three_taps_that_all_undershoot(t *testing.T) {
	fake := &walletFake{balances: []string{"0", "1", "2", "3"}}

	_, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)

	if err == nil || !strings.Contains(err.Error(), "still below 1000 Amulet after 3 taps") {
		t.Fatalf("error = %v, want one naming the shortfall after 3 taps", err)
	}
	if len(fake.tapBodies) != 3 {
		t.Fatalf("taps = %d, want exactly 3", len(fake.tapBodies))
	}
}

func TestTapAtLeast_fails_without_tapping_when_the_price_fetch_fails(t *testing.T) {
	fake := &walletFake{balances: []string{"0"}, roundsCode: http.StatusInternalServerError}

	_, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)

	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error = %v, want one naming HTTP 500", err)
	}
	if len(fake.tapBodies) != 0 {
		t.Fatalf("taps = %v, want none", fake.tapBodies)
	}
}

func TestTapAtLeast_fails_without_tapping_when_the_balance_fetch_fails(t *testing.T) {
	fake := &walletFake{balanceCode: http.StatusBadGateway}

	_, err := fundingClient(t, fake).TapAtLeast(context.Background(), "1000", fixedNow)

	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("error = %v, want one naming HTTP 502", err)
	}
	if len(fake.tapBodies) != 0 {
		t.Fatalf("taps = %v, want none", fake.tapBodies)
	}
}

func TestTapAtLeast_does_not_retry_a_5xx_tap(t *testing.T) {
	taps := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/validator/v0/wallet/balance":
			_, _ = w.Write([]byte(`{"effective_unlocked_qty":"0"}`))
		case "/api/validator/v0/scan-proxy/open-and-issuing-mining-rounds":
			_, _ = w.Write([]byte(miningRounds))
		default:
			taps++
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	_, err := wallet.Client{BaseURL: server.URL, Token: "t"}.TapAtLeast(context.Background(), "1000", fixedNow)

	if err == nil || taps != 1 {
		t.Fatalf("error = %v, taps = %d, want a failure after a single tap", err, taps)
	}
}

func TestTapAtLeast_rejects_an_invalid_target_before_any_request(t *testing.T) {
	fake := &walletFake{balances: []string{"0"}}

	for _, target := range []string{"0", "-1", "abc"} {
		if _, err := fundingClient(t, fake).TapAtLeast(context.Background(), target, fixedNow); err == nil {
			t.Fatalf("target %q: want an error", target)
		}
	}
	if fake.reads != 0 {
		t.Fatalf("balance reads = %d, want none", fake.reads)
	}
}

func TestAmuletPrice_fails_when_no_round_has_opened(t *testing.T) {
	fake := &walletFake{}
	early := func() time.Time { return time.Date(2026, 10, 3, 11, 0, 0, 0, time.UTC) }

	_, err := fundingClient(t, fake).AmuletPrice(context.Background(), early())

	if err == nil || !strings.Contains(err.Error(), "no open mining round") {
		t.Fatalf("error = %v, want one saying no round has opened", err)
	}
}

func TestAmuletPrice_picks_the_latest_round_that_has_opened(t *testing.T) {
	fake := &walletFake{}
	client := fundingClient(t, fake)

	before, err := client.AmuletPrice(context.Background(), fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	after, err := client.AmuletPrice(context.Background(), time.Date(2026, 10, 3, 11, 40, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if before != "0.005" || after != "0.01" {
		t.Fatalf("prices = %s then %s, want 0.005 then 0.01", before, after)
	}
}
