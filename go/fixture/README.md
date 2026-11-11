# canton-localnet Go fixture

For the full consumer guide, see `docs/public/integration-testing.md`.

Go-side integration-test harness for the Canton LocalNet compose stack. Pairs with the C# fixture; both target the same compose stack and OAuth2 realms.

Module path: `github.com/peacefulstudio/canton-localnet/go/fixture`. Go 1.23+.

## Surface

- `EndpointDiscovery` — resolves JSON Ledger API and Keycloak URLs from env vars.
- `OAuth2TokenProvider` — `client_credentials` grant with in-memory cache, concurrent-safe coalesced refresh, and a 30-second pre-expiry refresh skew.
- `JsonLedgerAdminClient` — pure `net/http` + `encoding/json` wrapper around the JSON Ledger Admin API. Exposes `GET /v2/parties/participant-id`, the connected-synchronizer lookups and a vetting lookup (`POST /v2/package-vetting/list`).
- `DarUploader` — `POST /v2/dars` per DAR, once per connected synchronizer with `?synchronizerId=` (a stack that lists none gets one upload without it), so it works on single-sync and multi-sync stacks. A transient `503` is retried with bounded backoff. HTTP 400 with `KNOWN_PACKAGE_VERSION` is success only when the DAR's own main package, read from its `META-INF/MANIFEST.MF`, is vetted on that synchronizer (checked through `POST /v2/package-vetting/list`); otherwise Canton rejected a different package id with the name and version of a vetted one, and `Upload` returns `*DarNotVettedError` naming the main package id and the ids Canton's error names. Re-uploading the same DAR is safe. `UploadAll` runs paths sequentially and stops at the first failure.
- `PartyAllocator` — `POST /v2/parties` with hints of the form `<prefix>-<suffix>`. The suffix is a random 16-character hex string generated once per allocator instance, so test runs sharing a ledger never collide. The party is allocated on the global synchronizer (resolved by alias, so it works on single-sync and multi-sync stacks); a transient `503`/`504`, `409 REQUEST_ALREADY_IN_FLIGHT` or request timeout is retried with backoff, and a party that an earlier attempt already created is found by its hint. `AllocateOnSynchronizer` targets an explicit synchronizer id.
- `UserBuilder` — a single `POST /v2/users` carrying the `actAs` / `readAs` rights; the rights array is empty when both lists are empty. Each user the fixture created is deleted on `Teardown` with one `DELETE /v2/users/{id}` and no `GET`: the first `Teardown` returns an error for any non-2xx response, and a retry of a failed `Teardown` counts a `404` with code `USER_NOT_FOUND` as success.
- `Fixture` — composes the above behind `Setup(ctx)` / `Teardown(ctx)` and exposes convenience methods `GetParticipantId`, `UploadDar`, `UploadDars`, `AllocateParty`, `CreateUser`. `CreateUser` records the user and `Teardown` deletes it, on the fixture and on every `Validator` view; a failed `Teardown` leaves the fixture set up, so calling it again retries only the users still outstanding.
- `ValidatorFixture` — per-slot view returned by `Fixture.Validator(role)` (or `Fixture.MustValidator(role)`). Exposes the same deep modules scoped to a single validator slot so tests that span multiple validators (e.g. allocate a party on `a-validator-1`, observe it on `b-validator-1`) don't have to juggle separate top-level fixtures. The first call for each role builds scoped clients; repeat calls return the cached instance. `KnownRoles()` returns the canonical slot set in stable order (sv, a, b, c, d).

## Usage

```go
import "github.com/peacefulstudio/canton-localnet/go/fixture"

func TestMyLedgerThing(t *testing.T) {
    ctx := context.Background()
    f, err := fixture.New(fixture.Config{Role: fixture.RoleAValidator1})
    if err != nil { t.Fatal(err) }
    if err := f.Setup(ctx); err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = f.Teardown(context.Background()) })

    if err := f.UploadDar(ctx, "testdata/my-package.dar"); err != nil {
        t.Fatal(err)
    }
    party, err := f.AllocateParty(ctx, "globex")
    if err != nil { t.Fatal(err) }

    _, err = f.CreateUser(ctx, fixture.UserOptions{
        UserID:       "alice",
        PrimaryParty: party,
        ActAs:        []string{party},
    })
    if err != nil { t.Fatal(err) }
}
```

### Multi-validator scenarios

Use `Validator(role)` to scope deep modules to a specific slot — handy
when a test allocates a party on one validator and verifies it from
another:

```go
f, _ := fixture.New(fixture.Config{Role: fixture.RoleAValidator1})
_ = f.Setup(ctx)
defer f.Teardown(ctx)

a := f.MustValidator(fixture.RoleAValidator1)
b := f.MustValidator(fixture.RoleBValidator1)

party, _ := a.AllocateParty(ctx, "shared")
// later, from B's perspective
id, _ := b.GetParticipantId(ctx)
_ = id
_ = party
```

## Environment variables

Every value has a default that matches `compose/modules/localnet/env/common.env` and `compose/modules/keycloak/compose.env`. Override per shell or per CI job. Resolution follows ADR-0003 and is shared with the C# fixture: for each value, the per-slot variable wins, then the host/port composition and global variables below, then the default. `<SLOT>` is `A_VALIDATOR_1`, `B_VALIDATOR_1`, `C_VALIDATOR_1`, `D_VALIDATOR_1` or `SV_VALIDATOR_1`.

| Variable | Default | Notes |
|---|---|---|
| `CANTON_LOCALNET_<SLOT>_JSON_API_URL` | `http://${CANTON_LOCALNET_HOST}:<port>` | JSON Ledger API base URL. Wins over host and port. |
| `CANTON_LOCALNET_<SLOT>_GRPC_URL` | `http://${CANTON_LOCALNET_HOST}:<port>` | gRPC Ledger API address, returned as `Endpoints.GRPCLedgerAPIURL`. Wins over host and port. |
| `CANTON_LOCALNET_<SLOT>_TOKEN_URL` | `http://${CANTON_LOCALNET_KEYCLOAK_HOST}:${CANTON_LOCALNET_KEYCLOAK_PORT}/realms/<realm>/protocol/openid-connect/token` | Keycloak `client_credentials` endpoint. Wins over Keycloak host and port. No default for sv. |
| `CANTON_LOCALNET_<SLOT>_CLIENT_ID` | `<slot>-validator`, e.g. `a-validator-1-validator` | Matches `AUTH_<SLOT>_VALIDATOR_CLIENT_ID`. No default for sv. |
| `CANTON_LOCALNET_<SLOT>_CLIENT_SECRET` | demo secret | Matches `AUTH_<SLOT>_VALIDATOR_CLIENT_SECRET`. No default for sv. |
| `CANTON_LOCALNET_<SLOT>_AUDIENCE` | `CANTON_LOCALNET_AUDIENCE`, else `https://canton.network.global` | Sent as the `audience` form field on token requests. |
| `CANTON_LOCALNET_<SLOT>_SCOPE` | `CANTON_LOCALNET_SCOPE`, else `openid` | Sent as the `scope` form field. |
| `CANTON_LOCALNET_HOST` | `localhost` | Go-only fallback: host of the JSON and gRPC Ledger APIs when `_JSON_API_URL` or `_GRPC_URL` is unset. |
| `CANTON_LOCALNET_<SLOT>_JSON_PORT` | `10975` (sv), `11975` (a), `12975` (b), `13975` (c), `14975` (d) | Go-only fallback: port when `_JSON_API_URL` is unset. `<prefix>${PARTICIPANT_JSON_API_PORT_SUFFIX}` in compose. |
| `CANTON_LOCALNET_<SLOT>_GRPC_PORT` | `10901` (sv), `11901` (a), `12901` (b), `13901` (c), `14901` (d) | Go-only fallback: port when `_GRPC_URL` is unset. `<prefix>${PARTICIPANT_LEDGER_API_PORT_SUFFIX}` in compose. |
| `CANTON_LOCALNET_KEYCLOAK_HOST` | `localhost` | Go-only fallback: Keycloak host when `_TOKEN_URL` is unset. |
| `CANTON_LOCALNET_KEYCLOAK_PORT` | `8082` | Go-only fallback: Keycloak port when `_TOKEN_URL` is unset. |
| `CANTON_LOCALNET_AUDIENCE` | `https://canton.network.global` | Fallback for every slot without a per-slot audience. |
| `CANTON_LOCALNET_SCOPE` | `openid` | Fallback for every slot without a per-slot scope. |

The Go fixture does not read the C# fixture's legacy un-namespaced globals (`CANTON_LOCALNET_JSON_API_URL`, `_TOKEN_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`) or `CANTON_LOCALNET_PROFILE`; the slot is the `Role` you pass.

**`RoleSvValidator1` has no defaults.** The keycloak module imports realms for A, B, C and D only, so there is no SV realm to derive a token URL from. `For(RoleSvValidator1)` returns an error naming `CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL`, `CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID` and `CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET` unless all three are set. The C# fixture behaves the same way.

> Security note — demo credentials only. The default `CLIENT_SECRET` values for `RoleAValidator1`, `RoleBValidator1`, `RoleCValidator1`, and `RoleDValidator1` are the demo credentials shipped with the splice quickstart compose files (also visible in `compose/modules/keycloak/env/`). They are public, valid only against an ephemeral LocalNet Keycloak realm, and **must never be used in any non-`localhost` or production deployment**. Override via the `CANTON_LOCALNET_*_CLIENT_SECRET` env vars in any real environment. `RoleSvValidator1` ships with no demo default: its token URL, client id and secret must be supplied explicitly.

These env vars are the **fixture / test config layer** — distinct from
`canton-localnet.yaml`, which configures the CLI's stack-boot
(`canton-localnet up`). The fixture connects to an already-running
stack and needs per-slot URLs / credentials, so env vars are the
discovery mechanism here.

## Tests

```
go test ./...                # unit
go test -tags integration ./...   # boots stack via compose; CI only
```

The PQS reader-role integration test lives in the nested module `tests/pqs-reader-role` so its Postgres driver stays out of this module's requirements; run it with `go test -tags integration ./...` from that directory.

The integration test is gated by `//go:build integration` and assumes the compose stack is already up and ready (`make up && make wait-ready`).
