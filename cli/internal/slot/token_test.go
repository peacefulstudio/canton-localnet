// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package slot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMintTokenOAuth2PostsClientCredentials(t *testing.T) {
	t.Parallel()
	var captured url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"abc","token_type":"Bearer","expires_in":300}`))
	}))
	defer srv.Close()

	ep := Endpoints{
		Slot:         Slot{Canonical: "a-validator-1", AuthKind: AuthKindOAuth2},
		TokenURLHost: srv.URL,
		ClientID:     "client",
		ClientSecret: "secret",
		Audience:     "https://canton.network.global",
	}
	got, err := MintToken(context.Background(), ep, srv.Client())
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	if got != "abc" {
		t.Errorf("token: got %q, want abc", got)
	}
	if captured.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type: got %q", captured.Get("grant_type"))
	}
	if captured.Get("client_id") != "client" {
		t.Errorf("client_id: got %q", captured.Get("client_id"))
	}
	if captured.Get("client_secret") != "secret" {
		t.Errorf("client_secret: got %q", captured.Get("client_secret"))
	}
	if captured.Get("audience") != "https://canton.network.global" {
		t.Errorf("audience: got %q", captured.Get("audience"))
	}
}

func TestMintTokenOAuth2SurfacesErrorBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	ep := Endpoints{
		Slot:         Slot{Canonical: "a-validator-1", AuthKind: AuthKindOAuth2},
		TokenURLHost: srv.URL,
		ClientID:     "c",
		ClientSecret: "s",
	}
	_, err := MintToken(context.Background(), ep, srv.Client())
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("expected error to surface status + body, got %q", err.Error())
	}
}

func TestMintTokenOAuth2RejectsNonJSON2xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>maintenance</html>"))
	}))
	defer srv.Close()

	ep := Endpoints{
		Slot:         Slot{Canonical: "a-validator-1", AuthKind: AuthKindOAuth2},
		TokenURLHost: srv.URL,
		ClientID:     "c",
		ClientSecret: "s",
	}
	_, err := MintToken(context.Background(), ep, srv.Client())
	if err == nil {
		t.Fatal("expected error decoding HTML body")
	}
	if !strings.Contains(err.Error(), "decode token response") {
		t.Errorf("expected decode-failure error, got %q", err.Error())
	}
}

func TestMintHS256ExpAndIatAreSane(t *testing.T) {
	t.Parallel()
	fixed := time.Unix(1_700_000_000, 0).UTC()
	ep := Endpoints{
		Slot:        Slot{Canonical: "sv-validator-1", AuthKind: AuthKindHS256},
		HS256Secret: "unsafe",
		HS256User:   "ledger-api-user",
		Audience:    "https://canton.network.global",
	}
	tok, err := mintHS256(ep, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Sub string `json:"sub"`
		Aud string `json:"aud"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iat != fixed.Unix() {
		t.Errorf("iat: got %d, want %d", claims.Iat, fixed.Unix())
	}
	if want := fixed.Add(HS256TTL).Unix(); claims.Exp != want {
		t.Errorf("exp: got %d, want %d", claims.Exp, want)
	}
	if claims.Exp <= claims.Iat {
		t.Errorf("exp (%d) must be > iat (%d)", claims.Exp, claims.Iat)
	}
}

func TestMintHS256ProducesVerifiableJWT(t *testing.T) {
	t.Parallel()
	ep := Endpoints{
		Slot:        Slot{Canonical: "sv-validator-1", AuthKind: AuthKindHS256},
		HS256Secret: "unsafe",
		HS256User:   "ledger-api-user",
		Audience:    "https://canton.network.global",
	}
	tok, err := MintToken(context.Background(), ep, nil)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected JWT with 3 parts, got %d in %q", len(parts), tok)
	}
	mac := hmac.New(sha256.New, []byte("unsafe"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if !hmac.Equal(mac.Sum(nil), expected) {
		t.Error("signature did not verify against secret")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "ledger-api-user" {
		t.Errorf("sub: got %v", claims["sub"])
	}
	if claims["aud"] != "https://canton.network.global" {
		t.Errorf("aud: got %v", claims["aud"])
	}
}
