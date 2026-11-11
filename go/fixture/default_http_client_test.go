// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultHTTPClients_HaveAThirtySecondTimeoutAndAreNotTheSharedDefaultClient(t *testing.T) {
	tokens := staticTokenProvider{token: "tok"}
	const baseURL = "http://localhost:11975"

	dar, err := NewDarUploader(baseURL, tokens)
	if err != nil {
		t.Fatalf("NewDarUploader: %v", err)
	}
	party, err := NewPartyAllocator(baseURL, tokens)
	if err != nil {
		t.Fatalf("NewPartyAllocator: %v", err)
	}
	user, err := NewUserBuilder(baseURL, tokens)
	if err != nil {
		t.Fatalf("NewUserBuilder: %v", err)
	}
	admin, err := NewJsonLedgerAdminClient(baseURL, tokens)
	if err != nil {
		t.Fatalf("NewJsonLedgerAdminClient: %v", err)
	}
	oauth, err := NewOAuth2TokenProvider(OAuth2TokenProviderConfig{
		TokenURL:     "http://localhost:8080/token",
		ClientID:     "id",
		ClientSecret: "sec",
	})
	if err != nil {
		t.Fatalf("NewOAuth2TokenProvider: %v", err)
	}

	clients := map[string]*http.Client{
		"DarUploader":           dar.httpClient,
		"PartyAllocator":        party.httpClient,
		"UserBuilder":           user.httpClient,
		"JsonLedgerAdminClient": admin.httpClient,
		"OAuth2TokenProvider":   oauth.cfg.HTTPClient,
	}
	for name, client := range clients {
		if client == http.DefaultClient {
			t.Errorf("%s uses http.DefaultClient", name)
		}
		if client.Timeout != 30*time.Second {
			t.Errorf("%s client timeout = %v, want 30s", name, client.Timeout)
		}
	}
}
