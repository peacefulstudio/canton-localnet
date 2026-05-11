# canton-localnet Go fixture

Go-side integration-test harness for the Canton LocalNet compose stack. Pairs with the C# fixture; both target the same compose stack and OAuth2 realms.

Module path: `github.com/peacefulstudio/canton-localnet/go/fixture`. Go 1.23+.

## Surface

- `EndpointDiscovery` — resolves JSON Ledger API and Keycloak URLs from env vars.
- `OAuth2TokenProvider` — `client_credentials` grant with in-memory cache, concurrent-safe coalesced refresh, and a 30-second pre-expiry refresh skew.
- `JsonLedgerAdminClient` — pure `net/http` + `encoding/json` wrapper around the JSON Ledger Admin API. Currently exposes `GET /v2/parties/participant-id`.
- `Fixture` — composes the above behind `Setup(ctx)` / `Teardown(ctx)`.

## Usage

```go
import "github.com/peacefulstudio/canton-localnet/go/fixture"

func TestMyLedgerThing(t *testing.T) {
    ctx := context.Background()
    f, err := fixture.New(fixture.Config{Role: fixture.RoleAppProvider})
    if err != nil { t.Fatal(err) }
    if err := f.Setup(ctx); err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = f.Teardown(context.Background()) })

    id, err := f.GetParticipantId(ctx)
    if err != nil { t.Fatal(err) }
    t.Logf("participant: %s", id)
}
```

## Environment variables

All have defaults that match `compose/modules/localnet/env/common.env` and `compose/modules/keycloak/compose.env`. Override per shell or per CI job.

| Variable | Default | Notes |
|---|---|---|
| `CANTON_LOCALNET_HOST` | `localhost` | Host that exposes the JSON Ledger API. |
| `CANTON_LOCALNET_APP_PROVIDER_JSON_PORT` | `3975` | `3${PARTICIPANT_JSON_API_PORT_SUFFIX}` in compose. |
| `CANTON_LOCALNET_APP_USER_JSON_PORT` | `2975` | `2${PARTICIPANT_JSON_API_PORT_SUFFIX}`. |
| `CANTON_LOCALNET_SV_JSON_PORT` | `4975` | `4${PARTICIPANT_JSON_API_PORT_SUFFIX}`. |
| `CANTON_LOCALNET_KEYCLOAK_HOST` | `keycloak.localhost` | Matches the host-exposed nginx-keycloak alias. |
| `CANTON_LOCALNET_KEYCLOAK_PORT` | `8082` | |
| `CANTON_LOCALNET_AUDIENCE` | `https://canton.network.global` | Sent as the `audience` form field on token requests. |
| `CANTON_LOCALNET_SCOPE` | _empty_ | Optional `scope` form field. |
| `CANTON_LOCALNET_APP_PROVIDER_CLIENT_ID` | `app-provider-validator` | Matches `AUTH_APP_PROVIDER_VALIDATOR_CLIENT_ID`. |
| `CANTON_LOCALNET_APP_PROVIDER_CLIENT_SECRET` | demo secret | Matches `AUTH_APP_PROVIDER_VALIDATOR_CLIENT_SECRET`. |
| `CANTON_LOCALNET_APP_USER_CLIENT_ID` | `app-user-validator` | |
| `CANTON_LOCALNET_APP_USER_CLIENT_SECRET` | demo secret | |
| `CANTON_LOCALNET_SV_CLIENT_ID` | `sv-validator` | |
| `CANTON_LOCALNET_SV_CLIENT_SECRET` | _empty_ | Required for `RoleSV` — discovery returns an error if unset. |

> ⚠️ **Security note — demo credentials only.** The default `CLIENT_SECRET` values for `RoleAppProvider` and `RoleAppUser` are the demo credentials shipped with the splice quickstart compose files (also visible in `compose/modules/keycloak/env/`). They are public, valid only against an ephemeral LocalNet Keycloak realm, and **must never be used in any non-`localhost` or production deployment**. Override via the `CANTON_LOCALNET_*_CLIENT_SECRET` env vars in any real environment. `RoleSV` ships with no demo default — its secret must be supplied explicitly.

## Tests

```
go test ./...                # unit
go test -tags integration ./...   # boots stack via compose; CI only
```

The integration test is gated by `//go:build integration` and assumes the compose stack is already up and ready (`make up && make wait-ready`).
