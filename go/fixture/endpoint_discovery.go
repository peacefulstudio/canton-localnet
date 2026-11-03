// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"fmt"
	"os"
	"strings"
)

// Role identifies a participant role in the LocalNet stack.
type Role string

const (
	// RoleAValidator1 is the a-validator-1 participant, host-exposed on the 11xxx port range.
	RoleAValidator1 Role = "a-validator-1"
	// RoleBValidator1 is the b-validator-1 participant, host-exposed on the 12xxx port range.
	RoleBValidator1 Role = "b-validator-1"
	// RoleCValidator1 is the c-validator-1 participant, host-exposed on the 13xxx port range.
	RoleCValidator1 Role = "c-validator-1"
	// RoleSvValidator1 is the super-validator participant, host-exposed on the 10xxx port range.
	RoleSvValidator1 Role = "sv-validator-1"
	// RoleDValidator1 is the d-validator-1 participant, host-exposed on the 14xxx port range.
	RoleDValidator1 Role = "d-validator-1"
)

// Endpoints groups the URLs and OAuth2 client credentials a fixture needs
// in order to talk to one participant of the LocalNet stack.
type Endpoints struct {
	Role             Role
	JSONLedgerAPIURL string
	TokenURL         string
	ClientID         string
	ClientSecret     string
	Audience         string
	Scope            string
}

// String formats the endpoints with ClientSecret redacted, so logging an
// Endpoints value never writes the secret.
func (e Endpoints) String() string {
	return fmt.Sprintf(
		"Endpoints{Role:%q, JSONLedgerAPIURL:%q, TokenURL:%q, ClientID:%q, ClientSecret:<redacted>, Audience:%q, Scope:%q}",
		e.Role, e.JSONLedgerAPIURL, e.TokenURL, e.ClientID, e.Audience, e.Scope)
}

// GoString implements fmt.GoStringer so %#v redacts ClientSecret as String does.
func (e Endpoints) GoString() string { return e.String() }

// EndpointDiscovery resolves LocalNet endpoints from environment variables
// following ADR-0003, falling back to the defaults baked into the compose
// stack.
//
// SECURITY: the default ClientSecret values for RoleAValidator1,
// RoleBValidator1, RoleCValidator1, and RoleDValidator1 are the demo
// credentials shipped with the splice quickstart compose files. They are
// public LocalNet test credentials valid only against an ephemeral local
// Keycloak realm — never use them in any non-localhost or production
// deployment. Override via the CANTON_LOCALNET_<SLOT>_CLIENT_SECRET env vars
// in any real environment.
//
// RoleSvValidator1 has no defaults for its token URL, client id or client
// secret, because the keycloak module ships realms for a, b, c and d only.
// For resolves it only when CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL,
// _CLIENT_ID and _CLIENT_SECRET are all set, and otherwise returns an error
// naming the ones that are missing.
//
// <SLOT> is the SCREAMING_SNAKE_CASE slot: A_VALIDATOR_1, B_VALIDATOR_1,
// C_VALIDATOR_1, D_VALIDATOR_1 or SV_VALIDATOR_1. Each value resolves in this
// order, first match wins:
//
//	JSON Ledger API URL
//	  CANTON_LOCALNET_<SLOT>_JSON_API_URL
//	  http://CANTON_LOCALNET_HOST:CANTON_LOCALNET_<SLOT>_JSON_PORT
//	  (host default "localhost"; port default 10975, 11975, 12975, 13975, 14975 for sv, a, b, c, d)
//	Token URL
//	  CANTON_LOCALNET_<SLOT>_TOKEN_URL
//	  http://CANTON_LOCALNET_KEYCLOAK_HOST:CANTON_LOCALNET_KEYCLOAK_PORT/realms/<realm>/protocol/openid-connect/token
//	  (host default "localhost", port default "8082", realm AValidator1 to DValidator1; none for sv)
//	Audience
//	  CANTON_LOCALNET_<SLOT>_AUDIENCE
//	  CANTON_LOCALNET_AUDIENCE
//	  "https://canton.network.global"
//	Scope
//	  CANTON_LOCALNET_<SLOT>_SCOPE
//	  CANTON_LOCALNET_SCOPE
//	  "openid"
//	Client id
//	  CANTON_LOCALNET_<SLOT>_CLIENT_ID
//	  "<slot>-validator" for a, b, c, d: a-validator-1-validator, b-validator-1-validator, and so on
//	Client secret
//	  CANTON_LOCALNET_<SLOT>_CLIENT_SECRET
//	  the LocalNet demo secret for a, b, c, d (see SECURITY above)
type EndpointDiscovery struct {
	getenv func(string) string
}

// NewEndpointDiscovery returns a discovery wired to the process environment.
func NewEndpointDiscovery() *EndpointDiscovery {
	return &EndpointDiscovery{getenv: os.Getenv}
}

// NewEndpointDiscoveryWithEnv returns a discovery that reads variables from
// the supplied lookup function. Intended for tests; in production wire to
// os.Getenv via NewEndpointDiscovery.
func NewEndpointDiscoveryWithEnv(lookup func(string) string) *EndpointDiscovery {
	if lookup == nil {
		lookup = os.Getenv
	}
	return &EndpointDiscovery{getenv: lookup}
}

const (
	defaultHost         = "localhost"
	defaultKeycloakHost = "localhost"
	defaultKeycloakPort = "8082"
	defaultAudience     = "https://canton.network.global"
	defaultScope        = "openid"

	aValidator1DemoClientSecret   = "AL8648b9SfdTFImq7FV56Vd0KHifHBuC"
	bcdValidator1DemoClientSecret = "6m12QyyGl81d9nABWQXMycZdXho6ejEX"
)

type slotDefaults struct {
	envPrefix    string
	jsonPort     string
	realm        string
	clientID     string
	clientSecret string
}

var slotDefaultsByRole = map[Role]slotDefaults{
	RoleAValidator1:  {"A_VALIDATOR_1", "11975", "AValidator1", "a-validator-1-validator", aValidator1DemoClientSecret},
	RoleBValidator1:  {"B_VALIDATOR_1", "12975", "BValidator1", "b-validator-1-validator", bcdValidator1DemoClientSecret},
	RoleCValidator1:  {"C_VALIDATOR_1", "13975", "CValidator1", "c-validator-1-validator", bcdValidator1DemoClientSecret},
	RoleDValidator1:  {"D_VALIDATOR_1", "14975", "DValidator1", "d-validator-1-validator", bcdValidator1DemoClientSecret},
	RoleSvValidator1: {"SV_VALIDATOR_1", "10975", "", "", ""},
}

// For resolves the endpoint set for a given role.
func (d *EndpointDiscovery) For(role Role) (Endpoints, error) {
	slot, ok := slotDefaultsByRole[role]
	if !ok {
		return Endpoints{}, fmt.Errorf("unknown role %q", string(role))
	}

	slotVar := func(name string) string { return "CANTON_LOCALNET_" + slot.envPrefix + "_" + name }

	host := d.lookupOr("CANTON_LOCALNET_HOST", defaultHost)
	jsonURL := d.lookupOr(slotVar("JSON_API_URL"),
		fmt.Sprintf("http://%s:%s", host, d.lookupOr(slotVar("JSON_PORT"), slot.jsonPort)))

	tokenURL := d.lookupOr(slotVar("TOKEN_URL"), d.defaultTokenURL(slot.realm))
	clientID := d.lookupOr(slotVar("CLIENT_ID"), slot.clientID)
	clientSecret := d.lookupOr(slotVar("CLIENT_SECRET"), slot.clientSecret)

	var missing []string
	if tokenURL == "" {
		missing = append(missing, slotVar("TOKEN_URL"))
	}
	if clientID == "" {
		missing = append(missing, slotVar("CLIENT_ID"))
	}
	if clientSecret == "" {
		missing = append(missing, slotVar("CLIENT_SECRET"))
	}
	if len(missing) > 0 {
		return Endpoints{}, fmt.Errorf(
			"endpoint discovery: %s must be set for role %s (no default ships in compose)",
			strings.Join(missing, ", "), role)
	}

	return Endpoints{
		Role:             role,
		JSONLedgerAPIURL: jsonURL,
		TokenURL:         tokenURL,
		ClientID:         clientID,
		ClientSecret:     clientSecret,
		Audience:         d.lookupOr(slotVar("AUDIENCE"), d.lookupOr("CANTON_LOCALNET_AUDIENCE", defaultAudience)),
		Scope:            d.lookupOr(slotVar("SCOPE"), d.lookupOr("CANTON_LOCALNET_SCOPE", defaultScope)),
	}, nil
}

func (d *EndpointDiscovery) defaultTokenURL(realm string) string {
	if realm == "" {
		return ""
	}
	return fmt.Sprintf("http://%s:%s/realms/%s/protocol/openid-connect/token",
		d.lookupOr("CANTON_LOCALNET_KEYCLOAK_HOST", defaultKeycloakHost),
		d.lookupOr("CANTON_LOCALNET_KEYCLOAK_PORT", defaultKeycloakPort),
		realm)
}

func (d *EndpointDiscovery) lookupOr(key, fallback string) string {
	v := d.getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
