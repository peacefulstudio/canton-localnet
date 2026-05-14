// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"errors"
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

// EndpointDiscovery resolves LocalNet endpoints from environment variables,
// falling back to the defaults baked into the compose stack.
//
// SECURITY: the default ClientSecret values for RoleAValidator1,
// RoleBValidator1, RoleCValidator1, and RoleDValidator1 are the demo
// credentials shipped with the splice quickstart compose files. They are
// public LocalNet test credentials valid only against an ephemeral local
// Keycloak realm — never use them in any non-localhost or production
// deployment. Override via the CANTON_LOCALNET_*_CLIENT_SECRET env vars
// in any real environment. RoleSvValidator1 has no demo default;
// CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET must be set explicitly.
//
// Environment variables consulted (with defaults):
//
//	CANTON_LOCALNET_HOST                       (default "localhost")
//	CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT     (default "11975")
//	CANTON_LOCALNET_B_VALIDATOR_1_JSON_PORT         (default "12975")
//	CANTON_LOCALNET_C_VALIDATOR_1_JSON_PORT         (default "13975")
//	CANTON_LOCALNET_SV_VALIDATOR_1_JSON_PORT               (default "10975")
//	CANTON_LOCALNET_D_VALIDATOR_1_JSON_PORT         (default "14975")
//	CANTON_LOCALNET_KEYCLOAK_HOST              (default "keycloak.localhost")
//	CANTON_LOCALNET_KEYCLOAK_PORT              (default "8082")
//	CANTON_LOCALNET_AUDIENCE                   (default "https://canton.network.global")
//	CANTON_LOCALNET_SCOPE                      (default "")
//	CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID     (default "a-validator-1-validator")
//	CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET (LocalNet demo default — see SECURITY note above)
//	CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID         (default "b-validator-1-validator")
//	CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET     (LocalNet demo default — see SECURITY note above)
//	CANTON_LOCALNET_C_VALIDATOR_1_CLIENT_ID         (default "c-validator-1-validator")
//	CANTON_LOCALNET_C_VALIDATOR_1_CLIENT_SECRET     (LocalNet demo default — see SECURITY note above)
//	CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID               (default "sv-validator")
//	CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET           (no default — required for RoleSvValidator1)
//	CANTON_LOCALNET_D_VALIDATOR_1_CLIENT_ID         (default "d-validator-1-validator")
//	CANTON_LOCALNET_D_VALIDATOR_1_CLIENT_SECRET     (LocalNet demo default — see SECURITY note above)
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

// For resolves the endpoint set for a given role.
func (d *EndpointDiscovery) For(role Role) (Endpoints, error) {
	host := d.lookupOr("CANTON_LOCALNET_HOST", "localhost")
	keycloakHost := d.lookupOr("CANTON_LOCALNET_KEYCLOAK_HOST", "keycloak.localhost")
	keycloakPort := d.lookupOr("CANTON_LOCALNET_KEYCLOAK_PORT", "8082")
	audience := d.lookupOr("CANTON_LOCALNET_AUDIENCE", "https://canton.network.global")
	scope := d.lookupOr("CANTON_LOCALNET_SCOPE", "")

	switch role {
	case RoleAValidator1:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", "11975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/AValidator1/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID", "a-validator-1-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET", "AL8648b9SfdTFImq7FV56Vd0KHifHBuC"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleBValidator1:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_B_VALIDATOR_1_JSON_PORT", "12975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/BValidator1/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID", "b-validator-1-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET", "6m12QyyGl81d9nABWQXMycZdXho6ejEX"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleCValidator1:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_C_VALIDATOR_1_JSON_PORT", "13975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/CValidator1/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_C_VALIDATOR_1_CLIENT_ID", "c-validator-1-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_C_VALIDATOR_1_CLIENT_SECRET", "6m12QyyGl81d9nABWQXMycZdXho6ejEX"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleDValidator1:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_D_VALIDATOR_1_JSON_PORT", "14975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/DValidator1/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_D_VALIDATOR_1_CLIENT_ID", "d-validator-1-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_D_VALIDATOR_1_CLIENT_SECRET", "6m12QyyGl81d9nABWQXMycZdXho6ejEX"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleSvValidator1:
		secret := d.lookupOr("CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET", "")
		if secret == "" {
			return Endpoints{}, errors.New("endpoint discovery: CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET must be set for RoleSvValidator1 (no demo default ships in compose)")
		}
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_SV_VALIDATOR_1_JSON_PORT", "10975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/sv-validator-1/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID", "sv-validator"),
			ClientSecret:     secret,
			Audience:         audience,
			Scope:            scope,
		}, nil
	default:
		return Endpoints{}, fmt.Errorf("unknown role %q", string(role))
	}
}

func (d *EndpointDiscovery) lookupOr(key, fallback string) string {
	v := d.getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
