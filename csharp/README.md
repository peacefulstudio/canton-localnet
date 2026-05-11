<!--
Copyright (c) 2026 Peaceful Studio OÜ
SPDX-License-Identifier: Apache-2.0
-->

# Peaceful.Canton.Localnet.Testing

xUnit fixtures for Canton LocalNet integration tests.

This v0 slice (issue #7) covers four sub-modules:

- `EndpointDiscovery` — env vars to URLs (JSON Ledger API, Keycloak token endpoint).
- `OAuth2TokenProvider` — `client_credentials` grant with in-memory cache + refresh.
- `JsonLedgerAdminClient` — thin `HttpClient` wrapper. v0 implements only `GET /v2/parties/participant-id`.
- `LocalnetFixture` — composes the above into a single user-facing surface.

DAR upload, party allocation, and user binding are explicitly out of scope for v0; they land in issue #9.

## Environment variables

The fixture reads these env vars (matching the compose stack ports in `compose/modules/localnet/env/common.env`):

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `CANTON_LOCALNET_PROFILE` | no | `app-provider` | One of `app-user`, `app-provider`, `sv`. |
| `CANTON_LOCALNET_JSON_API_URL` | no | `http://localhost:{2,3,4}975` per profile | Base URL of the JSON Ledger API. |
| `CANTON_LOCALNET_TOKEN_URL` | no | `http://localhost:8082/realms/{AppProvider,AppUser}/protocol/openid-connect/token` | Keycloak token endpoint. |
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
