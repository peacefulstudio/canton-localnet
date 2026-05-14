<!--
Copyright (c) 2026 Peaceful Studio OÜ
SPDX-License-Identifier: Apache-2.0
-->

# Peaceful.Canton.Localnet.Testing

xUnit fixtures for Canton LocalNet integration tests. Sub-modules:

- `EndpointDiscovery` — env vars to URLs (JSON Ledger API, Keycloak token endpoint).
- `OAuth2TokenProvider` — `client_credentials` grant with in-memory cache + refresh.
- `JsonLedgerAdminClient` — thin `HttpClient` wrapper. Currently surfaces `GET /v2/parties/participant-id`.
- `DarUploader` — `POST /v2/packages` for one DAR or many; idempotent: a
  `KNOWN_PACKAGE_VERSION` 400 response is treated as success.
- `PartyAllocator` — `POST /v2/parties` with hint
  `<consumer-prefix>-<instance-suffix>` where the suffix is a 12-hex
  cryptographic random per fixture instance.
- `UserBuilder` — `POST /v2/users` followed by `POST /v2/users/{id}/rights`
  to grant `CanActAs` / `CanReadAs`.
- `LocalnetFixture` — composes the above into a single user-facing surface
  via `Microsoft.Extensions.DependencyInjection` and exposes convenience
  pass-throughs (`UploadDarAsync`, `AllocatePartyAsync`, `CreateUserAsync`,
  `GetParticipantIdAsync`).

## 30-line example

```csharp
await using var fixture = LocalnetFixture.FromEnvironment();

await fixture.UploadDarAsync("./dars/my-workflow-1.0.0.dar");

var party = await fixture.AllocatePartyAsync("cdg");

await fixture.CreateUserAsync(
    userId: $"cdg-user-{fixture.PartyAllocator.InstanceSuffix}",
    primaryParty: party.PartyId,
    actAs: new[] { party.PartyId });
```

## Environment variables

The fixture reads these env vars (matching the compose stack ports in `compose/modules/localnet/env/common.env`):

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `CANTON_LOCALNET_PROFILE` | no | `a-validator-1` | One of `a-validator-1`, `b-validator-1`, `sv-validator-1`. |
| `CANTON_LOCALNET_JSON_API_URL` | no | `http://localhost:{11,12,10}975` per profile | Base URL of the JSON Ledger API. |
| `CANTON_LOCALNET_TOKEN_URL` | no | `http://localhost:8082/realms/{AValidator1,BValidator1}/protocol/openid-connect/token` | Keycloak token endpoint. |
| `CANTON_LOCALNET_AUDIENCE` | no | `https://canton.network.global` | Expected `aud` claim. |
| `CANTON_LOCALNET_CLIENT_ID` | **yes** | — | OAuth2 `client_id`. |
| `CANTON_LOCALNET_CLIENT_SECRET` | **yes** | — | OAuth2 `client_secret`. |
| `CANTON_LOCALNET_SCOPE` | no | `openid` | OAuth2 scope. |

The integration smoke test (`LocalnetSmokeTests`) self-skips when `CANTON_LOCALNET_JSON_API_URL` / `CLIENT_ID` / `CLIENT_SECRET` are not set, so unit tests run cleanly on a developer machine without `make up`.

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
  coverlet.runsettings           # coverage config shared with csharp-ci.yaml
  Peaceful.Canton.Localnet.Testing.sln
  src/Peaceful.Canton.Localnet.Testing/
  tests/Peaceful.Canton.Localnet.Testing.Tests/
```
