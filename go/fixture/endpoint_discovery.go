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
	// RoleAppProvider is the app-provider participant, host-exposed on the 3xxx port range.
	RoleAppProvider Role = "app-provider"
	// RoleAppUser is the app-user participant, host-exposed on the 2xxx port range.
	RoleAppUser Role = "app-user"
	// RoleSV is the super-validator participant, host-exposed on the 4xxx port range.
	RoleSV Role = "sv"
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
// SECURITY: the default ClientSecret values for RoleAppProvider and
// RoleAppUser are the demo credentials shipped with the splice quickstart
// compose files. They are public LocalNet test credentials valid only
// against an ephemeral local Keycloak realm — never use them in any
// non-localhost or production deployment. Override via the
// CANTON_LOCALNET_*_CLIENT_SECRET env vars in any real environment.
// RoleSV has no demo default; CANTON_LOCALNET_SV_CLIENT_SECRET must be
// set explicitly.
//
// Environment variables consulted (with defaults):
//
//	CANTON_LOCALNET_HOST                       (default "localhost")
//	CANTON_LOCALNET_APP_PROVIDER_JSON_PORT     (default "3975")
//	CANTON_LOCALNET_APP_USER_JSON_PORT         (default "2975")
//	CANTON_LOCALNET_SV_JSON_PORT               (default "4975")
//	CANTON_LOCALNET_KEYCLOAK_HOST              (default "keycloak.localhost")
//	CANTON_LOCALNET_KEYCLOAK_PORT              (default "8082")
//	CANTON_LOCALNET_AUDIENCE                   (default "https://canton.network.global")
//	CANTON_LOCALNET_SCOPE                      (default "")
//	CANTON_LOCALNET_APP_PROVIDER_CLIENT_ID     (default "app-provider-validator")
//	CANTON_LOCALNET_APP_PROVIDER_CLIENT_SECRET (LocalNet demo default — see SECURITY note above)
//	CANTON_LOCALNET_APP_USER_CLIENT_ID         (default "app-user-validator")
//	CANTON_LOCALNET_APP_USER_CLIENT_SECRET     (LocalNet demo default — see SECURITY note above)
//	CANTON_LOCALNET_SV_CLIENT_ID               (default "sv-validator")
//	CANTON_LOCALNET_SV_CLIENT_SECRET           (no default — required for RoleSV)
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
	case RoleAppProvider:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_APP_PROVIDER_JSON_PORT", "3975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/AppProvider/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_APP_PROVIDER_CLIENT_ID", "app-provider-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_APP_PROVIDER_CLIENT_SECRET", "AL8648b9SfdTFImq7FV56Vd0KHifHBuC"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleAppUser:
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_APP_USER_JSON_PORT", "2975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/AppUser/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_APP_USER_CLIENT_ID", "app-user-validator"),
			ClientSecret:     d.lookupOr("CANTON_LOCALNET_APP_USER_CLIENT_SECRET", "6m12QyyGl81d9nABWQXMycZdXho6ejEX"),
			Audience:         audience,
			Scope:            scope,
		}, nil
	case RoleSV:
		secret := d.lookupOr("CANTON_LOCALNET_SV_CLIENT_SECRET", "")
		if secret == "" {
			return Endpoints{}, errors.New("endpoint discovery: CANTON_LOCALNET_SV_CLIENT_SECRET must be set for RoleSV (no demo default ships in compose)")
		}
		return Endpoints{
			Role:             role,
			JSONLedgerAPIURL: fmt.Sprintf("http://%s:%s", host, d.lookupOr("CANTON_LOCALNET_SV_JSON_PORT", "4975")),
			TokenURL:         fmt.Sprintf("http://%s:%s/realms/sv/protocol/openid-connect/token", keycloakHost, keycloakPort),
			ClientID:         d.lookupOr("CANTON_LOCALNET_SV_CLIENT_ID", "sv-validator"),
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
