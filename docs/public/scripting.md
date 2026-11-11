<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Driving a slot from a script

Downstream apps that allocate parties, upload DARs, or otherwise drive
a slot's JSON Ledger API can stop hardcoding realm names, client
secrets, and the `<prefix>975` port scheme by going through two CLI
primitives:

```bash
# Mint a participant-admin bearer for the slot.
TOKEN=$(canton-localnet auth token --slot a)

# Get the slot's endpoints + live participant id as JSON.
canton-localnet info --slot a --json
```

`auth token` does an OAuth2 `client_credentials` exchange against the
slot's Keycloak realm for a/b/c/d, and mints a self-signed HS256 JWT
for sv-validator-1 (which doesn't run Keycloak). Resolution layers, from
highest to lowest priority:

1. `CANTON_LOCALNET_<SLOT>_CLIENT_ID` / `_CLIENT_SECRET` env vars
2. `canton-localnet.yaml` `validators.<slot>.auth.clientId` / `clientSecret` (the same fields `up` reads)
3. `compose/modules/keycloak/env/<slot>/on/oauth2.env` — so secret rotations in this repo land automatically
4. Built-in default

For sv-validator-1, the HS256 signing secret defaults to the LocalNet
shared `"unsafe"` string; override via
`CANTON_LOCALNET_SV_VALIDATOR_1_HS256_SECRET` if you have customised the
SV participant's auth config. The CLI does not validate the sv secret
locally — a wrong secret surfaces as a downstream 401 from the JSON
Ledger API. `validators.sv-validator-1.auth.clientId` / `clientSecret`
in `canton-localnet.yaml` are silently ignored because the HS256 path
doesn't have a client id.

`info --json` returns:

```json
{
  "slot": "a-validator-1",
  "json_api": "http://localhost:11975",
  "ledger_grpc": "localhost:11901",
  "admin_grpc": "localhost:11902",
  "validator_admin": "localhost:11903",
  "realm": "AValidator1",
  "token_url_host": "http://localhost:8082/realms/AValidator1/protocol/openid-connect/token",
  "token_url_internal": "http://nginx-keycloak:8082/realms/AValidator1/protocol/openid-connect/token",
  "audience": "https://canton.network.global",
  "auth_kind": "oauth2",
  "party_hint": "a-validator-1",
  "participant_id": "participant::1220...",
  "participant_namespace": "1220...",
  "validator_primary_party": "a-validator-1::1220..."
}
```

Pass `--offline` to skip the live participant-id lookup when you only
need the static endpoint mapping (e.g. before the stack is up). Short
slot names (`a`, `b`, `c`, `d`, `sv`) and canonical names
(`a-validator-1`, …) are both accepted.

