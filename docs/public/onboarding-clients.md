<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Onboarding clients

Each user-facing validator realm ships a confidential OAuth client whose
service account holds just enough `realm-management` roles
(`manage-users`, `view-users`, `query-users`) to manage users in that
realm only. Downstream apps that need to create users (sign-up flows,
integration test seeders) should use **this** client instead of the
master realm `admin/admin` admin — master-realm admin is a footgun (one
bug wipes the wrong realm) and breaks over plain HTTP via the
`nginx-keycloak` proxy because Keycloak's master realm defaults to
`sslRequired=external`.

| Realm          | Client ID                  | Dev secret env var                              |
|----------------|----------------------------|-------------------------------------------------|
| `AValidator1`  | `a-validator-1-onboarding` | `AUTH_A_VALIDATOR_1_ONBOARDING_CLIENT_SECRET`   |
| `BValidator1`  | `b-validator-1-onboarding` | `AUTH_B_VALIDATOR_1_ONBOARDING_CLIENT_SECRET`   |
| `CValidator1`  | `c-validator-1-onboarding` | `AUTH_C_VALIDATOR_1_ONBOARDING_CLIENT_SECRET`   |
| `DValidator1`  | `d-validator-1-onboarding` | `AUTH_D_VALIDATOR_1_ONBOARDING_CLIENT_SECRET`   |

Secrets are committed in `compose/modules/keycloak/env/{slot}/on/oauth2.env`
and re-surfaced in `compose/modules/localnet/env/{slot}-auth-on.env`. They
are **dev-only** — for production deployments, rotate them.

Quick check: mint a token, create a user, and verify cross-realm access
is denied. Substitute the slot's realm + client + secret env var.

```bash
TOKEN=$(curl -sf -X POST http://localhost:8082/realms/AValidator1/protocol/openid-connect/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'client_id=a-validator-1-onboarding' \
  -d "client_secret=${AUTH_A_VALIDATOR_1_ONBOARDING_CLIENT_SECRET}" \
  -d 'grant_type=client_credentials' \
  -d 'scope=openid' | jq -r .access_token)

curl -sf -X POST http://localhost:8082/admin/realms/AValidator1/users \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","enabled":true,"credentials":[{"type":"password","value":"abc123","temporary":false}]}' \
  -w '%{http_code}\n' -o /dev/null
# expect: 201

curl -s -X GET http://localhost:8082/admin/realms/BValidator1/users \
  -H "Authorization: Bearer $TOKEN" -w '%{http_code}\n' -o /dev/null
# expect: 403 (token is scoped to AValidator1 only)
```

