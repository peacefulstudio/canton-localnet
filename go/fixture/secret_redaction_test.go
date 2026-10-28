// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"fmt"
	"strings"
	"testing"
)

const redactionTestSecret = "hunter2-secret-literal"
const redactionTestToken = "cached-access-token-literal"

func redactionTestEndpoints() Endpoints {
	return Endpoints{
		Role:             RoleAValidator1,
		JSONLedgerAPIURL: "http://localhost:11975",
		TokenURL:         "https://keycloak.test/token",
		ClientID:         "client-id-literal",
		ClientSecret:     redactionTestSecret,
		Audience:         "aud-literal",
		Scope:            "openid",
	}
}

func redactionTestConfig() OAuth2TokenProviderConfig {
	return OAuth2TokenProviderConfig{
		TokenURL:     "https://keycloak.test/token",
		ClientID:     "client-id-literal",
		ClientSecret: redactionTestSecret,
		Audience:     "aud-literal",
		Scope:        "openid",
	}
}

func TestSecretRedaction_FormattingNeverContainsTheClientSecret(t *testing.T) {
	endpoints := redactionTestEndpoints()
	config := redactionTestConfig()
	provider := &OAuth2TokenProvider{cfg: config, cached: redactionTestToken}
	fixtureHandle := &Fixture{endpoints: endpoints, tokens: provider}
	validatorHandle := &ValidatorFixture{role: RoleAValidator1, endpoints: endpoints, tokens: provider}
	values := map[string]any{
		"*Fixture":                        fixtureHandle,
		"*ValidatorFixture":               validatorHandle,
		"*OAuth2TokenProvider":            provider,
		"Endpoints":                       endpoints,
		"*Endpoints":                      &endpoints,
		"OAuth2TokenProviderConfig":       config,
		"*OAuth2TokenProviderConfig":      &config,
		"[]Endpoints":                     []Endpoints{endpoints},
		"map[string]Endpoints":            map[string]Endpoints{"a": endpoints},
		"struct embedding the config":     struct{ Config OAuth2TokenProviderConfig }{config},
		"struct holding the config by *T": struct{ Config *OAuth2TokenProviderConfig }{&config},
	}
	for name, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			t.Run(name+" "+verb, func(t *testing.T) {
				got := fmt.Sprintf(verb, value)
				for _, secret := range []string{redactionTestSecret, redactionTestToken} {
					if strings.Contains(got, secret) {
						t.Fatalf("%s of %s leaked %q: %s", verb, name, secret, got)
					}
				}
			})
		}
	}
}

func TestSecretRedaction_EndpointsStringMarksSecretRedactedAndKeepsOtherFields(t *testing.T) {
	want := `Endpoints{Role:"a-validator-1", JSONLedgerAPIURL:"http://localhost:11975", TokenURL:"https://keycloak.test/token", ClientID:"client-id-literal", ClientSecret:<redacted>, Audience:"aud-literal", Scope:"openid"}`
	endpoints := redactionTestEndpoints()

	for name, got := range map[string]string{
		"String":   endpoints.String(),
		"GoString": endpoints.GoString(),
		"%+v":      fmt.Sprintf("%+v", &endpoints),
	} {
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}

func TestSecretRedaction_ConfigStringMarksSecretRedactedAndKeepsOtherFields(t *testing.T) {
	want := `OAuth2TokenProviderConfig{TokenURL:"https://keycloak.test/token", ClientID:"client-id-literal", ClientSecret:<redacted>, Audience:"aud-literal", Scope:"openid"}`
	config := redactionTestConfig()

	for name, got := range map[string]string{
		"String":   config.String(),
		"GoString": config.GoString(),
		"%+v":      fmt.Sprintf("%+v", &config),
	} {
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}
