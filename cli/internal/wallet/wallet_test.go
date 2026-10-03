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
	"testing"

	"github.com/peacefulstudio/canton-localnet/cli/internal/wallet"
)

func TestTap_posts_the_amount_with_the_bearer_to_the_wallet_tap_path(t *testing.T) {
	var method, path, auth, contentType, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		method, path, auth, contentType, body = r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(raw)
		_, _ = w.Write([]byte(`{"contract_id":"00abc"}`))
	}))
	defer server.Close()

	contractID, err := wallet.Client{BaseURL: server.URL, Token: "wallet-token"}.Tap(context.Background(), "10.50")
	if err != nil {
		t.Fatal(err)
	}

	if method != "POST" || path != "/api/validator/v0/wallet/tap" {
		t.Fatalf("request = %s %s, want POST /api/validator/v0/wallet/tap", method, path)
	}
	if auth != "Bearer wallet-token" {
		t.Fatalf("Authorization = %q, want %q", auth, "Bearer wallet-token")
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if body != `{"amount":"10.50"}` {
		t.Fatalf("body = %s, want {\"amount\":\"10.50\"}", body)
	}
	if contractID != "00abc" {
		t.Fatalf("contract id = %q, want 00abc", contractID)
	}
}

func TestTap_fails_loudly_on_a_non_2xx_and_never_prints_the_token(t *testing.T) {
	for _, status := range []int{400, 401, 403, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`precondition violated`))
		}))
		_, err := wallet.Client{BaseURL: server.URL, Token: "secret-wallet-token"}.Tap(context.Background(), "1")
		server.Close()
		if err == nil {
			t.Fatalf("status %d: want an error", status)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) || !strings.Contains(err.Error(), "precondition violated") {
			t.Fatalf("status %d: error %q must name the status and body", status, err)
		}
		if strings.Contains(err.Error(), "secret-wallet-token") {
			t.Fatalf("status %d: error leaks the token: %q", status, err)
		}
	}
}

func TestTap_rejects_a_2xx_without_a_contract_id(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if _, err := (wallet.Client{BaseURL: server.URL, Token: "t"}).Tap(context.Background(), "1"); err == nil {
		t.Fatal("want an error when the response carries no contract_id")
	}
}

func TestValidateAmount(t *testing.T) {
	cases := []struct {
		amount  string
		wantErr bool
	}{
		{"10", false},
		{"2.5", false},
		{"0.0000000001", false},
		{"0.00000000001", true},
		{"0", true},
		{"0.0", true},
		{"-1", true},
		{"+1", true},
		{"1e3", true},
		{"", true},
		{"abc", true},
		{"1.", true},
		{".5", true},
	}
	for _, c := range cases {
		err := wallet.ValidateAmount(c.amount)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateAmount(%q) error = %v, wantErr %v", c.amount, err, c.wantErr)
		}
	}
}

func TestTap_refuses_an_invalid_amount_before_any_request(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()

	if _, err := (wallet.Client{BaseURL: server.URL, Token: "t"}).Tap(context.Background(), "-5"); err == nil {
		t.Fatal("want an error")
	}
	if called {
		t.Fatal("an invalid amount must not reach the wallet")
	}
}

func TestMintToken_posts_the_password_grant_form(t *testing.T) {
	var method, contentType, form string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		method, contentType, form = r.Method, r.Header.Get("Content-Type"), string(raw)
		_, _ = w.Write([]byte(`{"access_token":"jwt-1"}`))
	}))
	defer server.Close()

	token, err := wallet.MintToken(context.Background(), wallet.Credentials{
		TokenURL: server.URL, ClientID: "a-validator-1-unsafe", Username: "a-validator-1", Password: "abc123",
	}, nil, wallet.Retry{})
	if err != nil {
		t.Fatal(err)
	}

	if method != "POST" || contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("request = %s %s", method, contentType)
	}
	const want = "client_id=a-validator-1-unsafe&grant_type=password&password=abc123&scope=openid&username=a-validator-1"
	if form != want {
		t.Fatalf("form = %s, want %s", form, want)
	}
	if token != "jwt-1" {
		t.Fatalf("token = %q, want jwt-1", token)
	}
}

func TestMintToken_fails_on_a_rejected_login_without_printing_the_password(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := wallet.MintToken(context.Background(), wallet.Credentials{
		TokenURL: server.URL, ClientID: "c", Username: "a-validator-1", Password: "hunter2",
	}, nil, wallet.Retry{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("error = %v, want one naming HTTP 401", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks the password: %q", err)
	}
}
