<!--
Copyright (c) 2026 Peaceful Studio OÜ
SPDX-License-Identifier: Apache-2.0
-->

# Peaceful.Canton.Localnet.Testing

For the full consumer guide, see `docs/public/integration-testing.md`.

xUnit fixtures for Canton LocalNet integration tests. Sub-modules:

- `EndpointDiscovery` — env vars to URLs (JSON Ledger API, gRPC Ledger API, Keycloak token endpoint).
- `OAuth2TokenProvider` — `client_credentials` grant with in-memory cache + refresh.
- `JsonLedgerAdminClient` — thin `HttpClient` wrapper: `GET /v2/parties/participant-id`,
  `GET /v2/state/connected-synchronizers` (participant-wide when no party is
  given), alias lookups for the global and app-provider synchronizers, and
  `POST /v2/package-vetting/list` to check whether this participant has
  vetted a package on a synchronizer.
- `DarUploader` — `POST /v2/dars` for one DAR or many, once per
  connected synchronizer (`?synchronizerId=`), so vetting keeps working on
  both single-sync and multi-sync stacks. A `KNOWN_PACKAGE_VERSION` 400
  response is success only when the DAR's own main package (the `Main-Dalf`
  id in its manifest) is vetted on that synchronizer, so
  an identical re-upload stays idempotent. When it is not vetted, Canton
  rejected a different package that shares the name and version of a vetted
  one, and `DarNotVettedException` names the main package id and the other
  ids Canton's error lists.
- `PartyAllocator` — `POST /v2/parties` with hint
  `<consumer-prefix>-<instance-suffix>` where the suffix is a 12-hex
  cryptographic random per fixture instance. Allocates on the global
  synchronizer by default, resolved by alias, unless a `synchronizerId` is
  passed explicitly.
- `UserBuilder` — `POST /v2/users` carrying the `CanActAs` / `CanReadAs`
  rights, `POST /v2/users/{id}/rights` to grant more later,
  `PATCH /v2/users/{id}/rights` to revoke them again, and `GrantRightsLeaseAsync` for a grant that hands the rights
  back on dispose. A participant caps a user at 1000 rights, so on a
  long-lived shared stack take the lease — see *User rights on a shared
  stack* in `docs/public/integration-testing.md` for what it owns, when its
  disposal throws, and why `RevokeRightsAsync` is not the teardown tool.
- `LocalnetFixture` — composes the above into a single user-facing surface
  via `Microsoft.Extensions.DependencyInjection` and exposes convenience
  pass-throughs (`UploadDarAsync`, `AllocatePartyAsync`, `CreateUserAsync`,
  `GetParticipantIdAsync`). `CreateUserAsync` records the user and
  `DisposeAsync` deletes it, on the fixture and on every per-slot view; see
  *Users the fixture created* in `docs/public/integration-testing.md`.
  `Validator(slot)` returns a per-slot view
  exposing the same deep modules scoped to a single validator (handy for
  cross-validator scenarios); `KnownSlots()` returns the canonical slot
  set in stable order (`sv-validator-1`, `a-validator-1`,
  `b-validator-1`, `c-validator-1`, `d-validator-1`).

## 30-line example

```csharp
await using var fixture = LocalnetFixture.FromEnvironment();

await fixture.UploadDarAsync("./dars/my-workflow-1.0.0.dar");

var party = await fixture.AllocatePartyAsync("globex");

await fixture.CreateUserAsync(
    userId: $"globex-user-{fixture.PartyAllocator.InstanceSuffix}",
    primaryParty: party.PartyId,
    actAs: new[] { party.PartyId });

await using var rights = await fixture.GrantUserRightsLeaseAsync(
    fixture.ValidatorUserId,
    actAs: new[] { party.PartyId });
```

The lease revokes on dispose, so the shared validator user does not silt up
to `TOO_MANY_USER_RIGHTS`. A failed revoke throws out of `DisposeAsync`,
which under `await using` can replace an in-flight assertion failure —
dispose explicitly when the test body has assertions of its own. Dispose the
lease before the fixture: disposing the fixture deletes `globex-user-…`, and a
deleted user takes its rights with it.

## Environment variables

The fixture reads these env vars (matching the compose stack ports in `compose/modules/localnet/env/common.env`):

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `CANTON_LOCALNET_PROFILE` | no | `a-validator-1` | One of `sv-validator-1`, `a-validator-1`, `b-validator-1`, `c-validator-1`, `d-validator-1`. |
| `CANTON_LOCALNET_JSON_API_URL` | no | `http://localhost:{10,11,12,13,14}975` per profile | Base URL of the JSON Ledger API. |
| `CANTON_LOCALNET_TOKEN_URL` | no for a/b/c/d, **yes** for sv | `http://localhost:8082/realms/{AValidator1,BValidator1,CValidator1,DValidator1}/protocol/openid-connect/token` per profile. The `SvValidator1` profile has **no default** — no SV realm is imported into Keycloak today (`compose/modules/keycloak/conf/data/` ships A/B/C/D only), so the SV slot's token URL (along with its client id and client secret) must be set explicitly, e.g. pointing at a custom realm you've added. | Keycloak token endpoint. |
| `CANTON_LOCALNET_<SLOT>_GRPC_URL` | no | `http://localhost:{10,11,12,13,14}901` per slot | Participant gRPC Ledger API, read into `LocalnetEndpoints.GrpcLedgerApi`. Per-slot only: there is no un-namespaced global, so `canton-localnet env` output and `CANTON_LOCALNET_PROFILE=c-validator-1` alone reach slot c's gRPC port. |
| `CANTON_LOCALNET_AUDIENCE` | no | `https://canton.network.global` | Expected `aud` claim. |
| `CANTON_LOCALNET_CLIENT_ID` | no for a/b/c/d, **yes** for sv | `{a,b,c,d}-validator-1-validator` per profile. The `SvValidator1` profile has no default. | OAuth2 `client_id`. |
| `CANTON_LOCALNET_CLIENT_SECRET` | no for a/b/c/d, **yes** for sv | The public demo secret of the slot's Keycloak realm, built into `EndpointDiscovery` and committed to this repository (one value for a-validator-1, one shared by b, c and d). The `SvValidator1` profile has no default. Valid only against the local demo stack: never rely on it anywhere else. | OAuth2 `client_secret`. |
| `CANTON_LOCALNET_SCOPE` | no | `openid` | OAuth2 scope. |

`LocalnetFixture.FromEnvironment()` resolves a/b/c/d with no credential variables set, falling back to the demo client id and secret above. The availability gate `EndpointDiscovery.IsLocalnetAvailable` does not count those defaults: the integration test (`LocalNetIntegrationTests`) self-skips when `CANTON_LOCALNET_JSON_API_URL` / `CLIENT_ID` / `CLIENT_SECRET` (or their per-slot forms) are not set, so unit tests run cleanly on a developer machine without `make up`.

These env vars are the **fixture / test config layer** — distinct from
`canton-localnet.yaml`, which configures the CLI's stack-boot
(`canton-localnet up`). The fixture connects to an already-running
stack and needs per-slot URLs / credentials, so env vars are the
discovery mechanism here.

## Run

```bash
dotnet test --filter "Category!=Integration"             # unit tests only
dotnet test                                              # all (smoke skipped unless env set)
dotnet pack src/Peaceful.Canton.Localnet.Testing -c Release
```

## Layout

```
csharp/
  Directory.Build.props          # net10.0, nullable, treat warnings as errors
  Directory.Packages.props       # central package versions
  NuGet.config                   # pins source to nuget.org with mapping
  coverage.settings.xml          # MTP code-coverage config shared with csharp-ci.yaml
  Peaceful.Canton.Localnet.Testing.sln
  src/Peaceful.Canton.Localnet.Testing/
  tests/Peaceful.Canton.Localnet.Testing.Tests/
```
