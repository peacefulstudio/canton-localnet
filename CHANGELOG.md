# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Per-realm onboarding service-account client (`{slot}-onboarding`) on
  each user-facing validator realm (`AValidator1`, `BValidator1`,
  `CValidator1`, `DValidator1`), with `realm-management` client roles
  `manage-users`, `view-users`, `query-users` scoped to its realm only
  (#65). Removes the need for master-realm `admin/admin` admin in
  downstream user-creation flows (sign-up UI, integration test
  seeders). Dev secrets are surfaced via
  `AUTH_{SLOT}_ONBOARDING_CLIENT_ID` and
  `AUTH_{SLOT}_ONBOARDING_CLIENT_SECRET` in
  `compose/modules/keycloak/env/{slot}/on/oauth2.env` and
  `compose/modules/localnet/env/{slot}-auth-on.env`.
- Restart-survival acceptance test for the onboarding clients
  (`tests/acceptance/restart-survival.sh` + `make test-restart-survival`)
  that seeds a sentinel user via each slot's `{slot}-onboarding`
  client, drives a `canton-localnet down && canton-localnet up` cycle
  (volumes preserved), and re-verifies the sentinel survives — wired
  into the `warm-restart` CI scenario in
  `.github/workflows/integration-tests.yaml`.

### Changed

### Fixed

- Scribe (PQS) image pin bumped from `0.6.11` to `0.6.13` in
  `compose/modules/pqs/compose.env`. Scribe `0.6.11` crash-loops on
  any participant carrying a Daml LF 2.2 package with
  `java.util.NoSuchElementException: key not found: LanguageMinorVersion(2)`,
  which surfaced as `pqs-a-validator-1` failing to start once Splice's
  own onboarding wrote LF 2.2 events into the ledger stream. The
  override is still honoured (`SCRIBE_VERSION=… canton-localnet up`),
  so downstream consumers can pin back if needed.

## [0.6.2-4] - 2026-05-16

### Breaking

- **Slot rename — `sv` → `sv-validator-1`, `app-provider` → `a-validator-1`,
  `app-user` → `b-validator-1`** (#40). The rename applies to every form
  of the identifier across the artifact:
  - **Kebab / lowercase** (compose profile names, file paths under
    `compose/modules/localnet/conf/{canton,splice,console}/<slot>/`,
    service / container names like `wallet-web-ui-<slot>` /
    `ans-web-ui-<slot>` / `pqs-<slot>`, Keycloak client IDs like
    `app-provider-validator` → `a-validator-1-validator`).
  - **SCREAMING_SNAKE** (env var prefixes): `APP_PROVIDER_PARTY_HINT` →
    `A_VALIDATOR_1_PARTY_HINT`; `APP_USER_PARTY_HINT` →
    `B_VALIDATOR_1_PARTY_HINT`; `SV_PROFILE` → `SV_VALIDATOR_1_PROFILE`;
    `APP_PROVIDER_PROFILE` → `A_VALIDATOR_1_PROFILE`;
    `APP_USER_PROFILE` → `B_VALIDATOR_1_PROFILE`;
    `AUTH_APP_PROVIDER_*` → `AUTH_A_VALIDATOR_1_*`;
    `AUTH_APP_USER_*` → `AUTH_B_VALIDATOR_1_*`;
    `AUTH_SV_*` → `AUTH_SV_VALIDATOR_1_*`;
    `CANTON_LOCALNET_APP_PROVIDER_*` → `CANTON_LOCALNET_A_VALIDATOR_1_*`;
    `CANTON_LOCALNET_APP_USER_*` → `CANTON_LOCALNET_B_VALIDATOR_1_*`;
    `CANTON_LOCALNET_SV_*` → `CANTON_LOCALNET_SV_VALIDATOR_1_*`;
    `PQS_APP_PROVIDER_PROFILE` → `PQS_A_VALIDATOR_1_PROFILE`;
    `PQS_APP_USER_PROFILE` → `PQS_B_VALIDATOR_1_PROFILE`;
    `PQS_SV_PROFILE` → `PQS_SV_VALIDATOR_1_PROFILE`.
  - **PascalCase** (Keycloak realm names): `AppProvider` → `AValidator1`;
    `AppUser` → `BValidator1`. C# enum `LocalnetProfile.AppProvider` →
    `LocalnetProfile.AValidator1`; `LocalnetProfile.AppUser` →
    `LocalnetProfile.BValidator1`; `LocalnetProfile.Sv` →
    `LocalnetProfile.SvValidator1`.
  - **Go** (`Role` constants in `go/fixture`): `RoleAppProvider` →
    `RoleAValidator1`; `RoleAppUser` → `RoleBValidator1`; `RoleSV` →
    `RoleSvValidator1`.
  - **CLI** (compose profile names emitted by `cli/internal/compose`):
    `--profile app-provider` → `--profile a-validator-1`;
    `--profile app-user` → `--profile b-validator-1`;
    `--profile sv` → `--profile sv-validator-1`;
    `--profile pqs-app-provider` → `--profile pqs-a-validator-1`.
- **5-digit port scheme** (ADR-0002, #40). Host-exposed ports follow a
  two-digit prefix per slot (`sv-validator-1`=10, `a-validator-1`=11,
  `b-validator-1`=12) plus the existing 3-digit suffix:
  - Participant ledger API: `3901`/`2901`/`4901` →
    `11901`/`12901`/`10901`.
  - Participant admin API: `3902`/`2902`/`4902` →
    `11902`/`12902`/`10902`.
  - Participant JSON API: `3975`/`2975`/`4975` →
    `11975`/`12975`/`10975`.
  - Splice validator admin: `3903`/`2903`/`4903` →
    `11903`/`12903`/`10903`.
  - UIs: `3000`/`2000`/`4000` → `11000`/`12000`/`10000`.
  - Canton-internal ports (5008 sequencer, 5009 mediator, 5012 scan,
    etc.) are unchanged.
- New foundational docs ship with this release: `CONTEXT.md` (domain
  glossary), `docs/adr/0001-yaml-config-without-codegen.md`,
  `docs/adr/0002-two-digit-port-prefix.md`, and `docs/MIGRATION.md`
  (terse before/after table for consumer migration).

### Added

- `.github/workflows/integration-tests.yaml` (#45) — end-to-end CI
  integration test for the 5-slot topology. Three scenarios run in
  parallel on `ubuntu-latest`: `5-healthy-validators` brings up the
  default stack and polls `/readyz` on every validator's JSON Ledger API
  (`sv`/`a`/`b`/`c`/`d` at port prefixes 10/11/12/13/14);
  `3-healthy-validators` writes a `canton-localnet.yaml` setting
  `c-validator-1.enabled: false` and `d-validator-1.enabled: false`,
  runs `canton-localnet up`, then asserts no `c-validator-1` /
  `d-validator-1` container is running while `sv` / `a` / `b` reach
  `/readyz`; `warm-restart` runs `canton-localnet down` (preserving
  volumes) followed by `canton-localnet up` to re-attach to existing
  state — the regression test for the splice restart-loop diagnostic.
  The initial bring-up is wrapped in one retry to absorb the splice
  SV-validator bootstrap race on slow runners (capturing diagnostics
  before the retry); the warm-restart step itself is single-shot
  because it IS the test signal. Each scenario tears down with
  `--volumes` on completion and uploads `docker ps` + compose logs as
  an artifact on failure.
- Multi-validator fixture API (#44). `LocalnetFixture.Validator(slot)`
  (C#) and `Fixture.Validator(role)` (Go) return a per-slot view
  exposing the same deep modules (admin client, DAR uploader, party
  allocator, user builder) scoped to a single validator slot. Tests
  that span multiple validators — e.g. allocate a party on
  `a-validator-1`, observe it on `b-validator-1` — no longer have to
  juggle separate top-level fixtures. The first call for each slot
  resolves endpoints and builds scoped clients; repeat calls return
  the cached instance. Passing the fixture's default slot reuses the
  fixture's root clients so the two surfaces stay consistent. C# also
  exposes `LocalnetFixture.KnownSlots()` and Go exposes
  `fixture.KnownRoles()` returning the canonical five slots in stable
  order (sv, a, b, c, d).
- Validator slot `d-validator-1` at port prefix 14 (#42).
- Validator slot `c-validator-1` at port prefix 13 (#41)
- `canton-localnet.yaml` consumer config (preview-1, unstable) —
  walk-up discovery and env translation (#43). The CLI walks up from
  the working directory looking for a `canton-localnet.yaml` (matching
  `git` / `docker compose` / `kubectl` semantics), or accepts an
  explicit `--config <path>`. With no file found anywhere, the CLI
  falls back to built-in defaults (all five slots enabled, obs + pqs
  on, party hints equal to slot names). Partial configs merge over
  the defaults; references to unknown slots are a hard error. The
  parser emits env vars (`<SLOT>_PROFILE`, `<SLOT>_PARTY_HINT`,
  `<SLOT>_OAUTH_CLIENT_ID`, `<SLOT>_OAUTH_CLIENT_SECRET`,
  `OBS_PROFILE`, `PQS_PROFILE`) that the existing compose pipeline
  already consumes. Schema documented in
  `docs/canton-localnet-yaml-schema.md`.

### Changed

- CI cost-reduction batch (this PR). Seven changes shipped together to
  cut GitHub Actions consumption ~50% without changing CI infrastructure:
  (1) every PR workflow (`integration-tests.yaml`, `cli.yml`,
  `go-fixture.yaml`, `csharp.yml`, `go-ci.yaml`, `terraform-ci.yaml`)
  now declares
  `concurrency.group: ${{ github.workflow }}-${{ github.ref }}` with
  `cancel-in-progress: true`, so superseded push/PR runs cancel
  immediately instead of finishing in parallel. (2) `go-fixture.yaml`'s
  unit job drops macOS on PRs via a `fromJSON(github.event_name ==
  'push' && '["ubuntu-latest","macos-latest"]' || '["ubuntu-latest"]')`
  matrix expression. (3) `cli.yml`'s `build` job applies the same
  conditional matrix. (4) `csharp.yml` passes the same conditional
  expression for `os-list` to the reusable
  `peacefulstudio/github-actions` csharp-ci workflow, so PRs run only
  ubuntu while `push` to dev still exercises ubuntu/macos/windows.
  (5) New consolidated `.github/workflows/compose-integration.yaml`
  boots LocalNet once and runs the CLI `wait-ready` probe, the Go
  `-tags integration` tests, and the C# `Category=Integration` xUnit
  run sequentially against the same stack, replacing three duplicate
  "boot + smoke" jobs in `cli.yml` (`integration-up-wait-ready`),
  `go-fixture.yaml` (`integration`), and `csharp.yml` (`integration`).
  Path filters in those three workflows now scope to their own
  source — `cli/**`, `go/fixture/**`, `csharp/**` — so `compose/**`
  changes no longer fan out to every per-language workflow.
  (6) `integration-tests.yaml` gains a `setup` job that emits a JSON
  scenario list consumed by the `scenario` job's matrix via
  `fromJSON(needs.setup.outputs.scenarios)`. On `push` to dev or PRs
  carrying the `ci:full` label all 5 scenarios run; PRs without the
  label default to just `5-healthy-validators`. (7) The same `setup`
  job uses `dorny/paths-filter@v3` (SHA-pinned) to layer in extra
  scenarios on PRs: `observability-on` / `observability-off` when
  `compose/modules/observability/**` changes; `3-healthy-validators`
  and `warm-restart` when `cli/**` / `compose/**` / `Makefile`
  changes. The basic `5-healthy-validators` still runs on every
  triggering event.
- Docs refresh for the 5-validator topology and YAML config layer
  (this PR). `README.md`'s JSON Ledger API port table now lists all
  five slots (`sv`, `a`, `b`, `c`, `d`); the `vm tunnel` line spells
  out which service each forwarded port belongs to and notes that the
  default port set lags ADR-0002's 5-digit renumbering for legacy
  tunnel-script compatibility. The Go and C# fixture READMEs gain
  entries for the `c-validator-1` and `d-validator-1` env vars and
  explicitly frame fixture env vars as the test-config layer distinct
  from `canton-localnet.yaml`. `CONTEXT.md` no longer describes the
  5-slot topology as "next iteration", and `docs/MIGRATION.md` calls
  out that `c`/`d` are new slots without a "before" form. The README
  YAML example and the `docs/canton-localnet-yaml-schema.md` example
  both have their `clientId` updated from the legacy
  `app-provider-validator` to `a-validator-1-validator` (the latter
  caught in pre-flight review — the schema-doc example would have
  produced auth failures for any user copy-pasting it, since no
  `app-provider-validator` client exists in the Keycloak realm).
  `csharp/README.md`'s `CANTON_LOCALNET_TOKEN_URL` row and
  `go/fixture/README.md`'s `CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET`
  row both now explain that the `SvValidator1` / `RoleSvValidator1`
  profile derives a `realms/sv-validator-1` URL but no SV realm is
  currently imported into Keycloak (only A/B/C/D realms ship), so the
  URL 404s at runtime — users must either override
  `CANTON_LOCALNET_TOKEN_URL` (C#) or supply an SV realm import.
  `CONTRIBUTING.md` Go version updated to match the current `go.mod`
  pins (fixture 1.23+, CLI 1.26+).
- Consolidated `.github/workflows/compose-ci.yaml` into
  `.github/workflows/integration-tests.yaml` as new
  `observability-on` / `observability-off` scenarios and deleted the
  standalone `compose-ci.yaml` (#53). The same one-shot bring-up retry
  that rescued the `warm-restart` scenario now wraps every initial
  `make up` / `canton-localnet up` invocation across
  `integration-tests.yaml` and `cli.yml`, absorbing the splice
  SV-validator bootstrap race on slow runners. The `cli.yml` `smoke
  (up + wait-ready + down)` job is renamed to
  `integration-up-wait-ready` and its diagnostics artifact follows
  suit (`smoke-diagnostics` → `integration-up-wait-ready-diagnostics`).

### Fixed

- `cli/internal/tunnel/tunnel.go` package and `DefaultPorts`
  doc-comments updated: port `8082` is the Keycloak (`nginx-keycloak`)
  endpoint, not a "validator wallet UI"; `7575` is now described as
  the in-container Splice JSON Ledger API port with a cross-reference
  to the host-side `11975` under ADR-0002. The default port set value
  (`[11901, 7575, 8082]`) is unchanged — `vm tunnel`'s defaults still
  mirror the legacy `tunnel.sh` scripts in `murmures` and
  `terraform-provider-canton`; only the doc-comments describing the
  set were stale.
- `LocalnetFixture_returns_distinct_participant_ids_per_validator_slot`
  (C#) now gates on a new `EndpointDiscovery.IsSlotAvailable(profile)`
  helper for both a-validator-1 and b-validator-1 (was: only the default
  profile via `IsLocalnetAvailable()`), so the test skips cleanly when
  the b-validator side of the stack isn't reachable instead of failing
  noisily. Matches the Go counterpart's per-slot `skipIfStackUnreachable`
  gating. The `SkipMessage` constant is refreshed to mention the per-slot
  `CANTON_LOCALNET_<SLOT>_*` variable shape alongside the legacy globals.
- Multi-validator fixture routing (#52). `LocalnetFixture.Validator(slot)`
  (C#) and `Fixture.Validator(role)` (Go) now route per-slot endpoints
  through a slot-namespaced discovery surface — every override variable is
  prefixed with the canonical slot (`CANTON_LOCALNET_A_VALIDATOR_1_*`,
  `CANTON_LOCALNET_B_VALIDATOR_1_*`, etc.) and defaults derive from the
  slot's two-digit port prefix (ADR-0002). The C# bug was that
  `EndpointDiscovery.Resolve(profile)` read the legacy un-namespaced globals
  (`CANTON_LOCALNET_JSON_API_URL`, `CANTON_LOCALNET_CLIENT_ID`,
  `CANTON_LOCALNET_CLIENT_SECRET`, …) regardless of which profile it was
  resolving, so when integration CI set those to a-validator-1's endpoints
  every per-slot view collapsed onto a-validator-1 and
  `Validator("a-validator-1").GetParticipantIdAsync()` ==
  `Validator("b-validator-1").GetParticipantIdAsync()`. Fix: a new
  `EndpointDiscovery.ResolveForSlot(profile)` ignores those legacy globals
  entirely; `LocalnetFixture.BuildValidator(non-default-slot)` calls it
  instead of `Resolve`. Legacy globals still apply to the fixture's selected
  default profile (single-validator consumers see no behaviour change).
  The Go side never had this bug — its `EndpointDiscovery.For` already used
  per-slot env vars exclusively — but `TestSmoke_MultiValidator_GetParticipantIdPerSlot`
  is upgraded to assert `participantId(a) != participantId(b)` so a
  regression at that layer would also fail CI. Discovery shape decision
  recorded in [ADR-0003](docs/adr/0003-per-slot-endpoint-discovery.md);
  the previously-skipped
  `LocalnetFixture_returns_distinct_participant_ids_per_validator_slot`
  smoke test is re-enabled.
- `.github/workflows/csharp.yml` now references the
  `peacefulstudio/github-actions` reusable CSharp CI workflow at
  `@v1` instead of an unreachable 40-char SHA pin. The pinned commit
  was no longer reachable from a branch in the action repo, so
  GitHub Actions failed every csharp run with "workflow was not
  found" before any job dispatched. Matches the repo convention
  already used by `go-ci.yaml` and `claude.yaml` (mutable major tag
  for first-party actions; SHA pinning is retained for third-party
  step actions where supply-chain risk is real).
- `canton-localnet up` now honours the YAML config's per-slot
  `enabled` flag (#45). The CLI's compose pipeline previously
  hardcoded the `sv`/`a`/`b`/`d` profile list and gated `c` on a
  `C_VALIDATOR_1_PROFILE=on` env var, so `canton-localnet.yaml`
  disabling `c-validator-1` or `d-validator-1` had no effect on which
  containers came up. `compose.Options` now carries an
  `EnabledSlots []string` that the CLI populates from
  `yamlconfig.Config.EnabledSlots()`; `compose.Build` emits one
  `--profile <slot>` per enabled slot (sv is always added). The
  `pqs-c-validator-1` profile follows c's enablement uniformly — no
  more env-var indirection.
- Top-level `Makefile` `PROFILES` list now includes
  `--profile c-validator-1` (#45). PR #48 introduced the
  `c-validator-1` compose profile but did not update the Makefile's
  hardcoded profile list, so `make up` silently omitted the
  c-validator stack. Surfaced by the new 5-validator integration test.
- `cli/internal/health.WaitReady` now preserves the last observed HTTP
  status in its timeout error even when a later probe ends in a
  transport error (e.g. context-deadline-exceeded as the overall
  budget expires). Previously the most recent probe's error
  unconditionally overwrote the diagnostic, so seeing a 503 followed
  by a deadline-exceeded would report only the deadline. Also
  de-flaked `TestWaitReadyTimeout` on macOS-arm64, where this race
  was reliably reproducible.

### Notes

- The `canton-localnet.yaml` schema (`schemaVersion: preview-1`) is
  **preview / unstable** until compose codegen lands (per
  [ADR-0001](docs/adr/0001-yaml-config-without-codegen.md)). Breaking
  schema changes are allowed in this window and will be called out
  in subsequent CHANGELOG entries.

## [0.6.2-3] - 2026-05-14

### Fixed

- Removed unused `xunit.v3` PackageReference from
  `Peaceful.Canton.Localnet.Testing.csproj` (it was unused in the
  library source and leaked as a transitive runtime dependency on
  the published nupkg, causing
  `CS0433 The type 'Assert' exists in both 'xunit.assert' and 'xunit.v3.assert'`
  in consumer projects pinned to xunit v2). Discovered while wiring
  the package into the `murmures` test suite, which uses xunit 2.
  (This section is reconstructed retroactively in v0.6.2-4 — the
  promotion step was skipped at `v0.6.2-3` tag time, so the bullet
  sat in `[Unreleased]` and was extracted into the GitHub Release
  body for `v0.6.2-3` as-is.)

## [0.6.2-2] - 2026-05-14

### Fixed

- `release.yml` now creates the GitHub Release as a draft with all
  assets attached, then transitions it to a published release via
  `gh release edit --draft=false`. This works around GitHub's
  organization-level immutable-releases enforcement, which locks a
  release on creation when published directly and caused v0.6.2-1's
  `publish-release` job to fail at the asset-upload step with
  `Cannot upload assets to an immutable release`. Drafts are mutable;
  flipping `draft=false` after upload is the documented workaround.
  v0.6.2-1's NuGet package, Go module, and OCI compose artifact are
  valid and published — only the GitHub Release asset attachment
  failed on that tag. v0.6.2-2 supersedes v0.6.2-1 as the first
  complete release on splice 0.6.2; the tag `v0.6.2-1` cannot be
  reused for a GitHub Release because GitHub retains its immutable
  release record.

## [0.6.2-1] - 2026-05-14

### Added

- Tag-driven release pipeline (#11) — `.github/workflows/release.yml`
  triggered on `v<splice>-<patch>` tag pushes (e.g. `v0.6.2-1`).
  Produces four artifacts in lockstep on every tag: the
  `Peaceful.Canton.Localnet.Testing.<version>.nupkg` pushed to the
  GitHub Packages NuGet feed; the Go module version reachable via
  `go get github.com/peacefulstudio/canton-localnet/go/fixture@<version>`
  (the tag itself is the module version); multi-platform CLI
  binaries (`linux/amd64`, `linux/arm64`, `darwin/amd64`,
  `darwin/arm64`, `windows/amd64`) attached to the GitHub Release
  with a SHA-256 `checksums.txt`, embedding the version via
  `-ldflags -X main.version=<version>` so `canton-localnet --version`
  reports it; and the `compose/` directory packaged via `oras` and
  pushed to `ghcr.io/peacefulstudio/canton-localnet:<version>`.
  Release notes are extracted from `CHANGELOG.md`'s `[Unreleased]`
  section verbatim. The version-format rule (never a plain
  `v<splice>`, patch resets on splice bump) and the release SOP are
  documented in the new [`RELEASE.md`](RELEASE.md), linked from
  `CONTRIBUTING.md`.
- `canton-localnet vm` parent command with three subcommands (issue
  #15) — `provision` runs `terraform init` + `terraform apply
  -auto-approve` against `terraform/` and prints the elastic IP plus a
  ready-to-paste ssh command on success (re-running is a no-op
  terraform refresh); `destroy` runs `terraform destroy
  -auto-approve` behind an interactive `Type DESTROY` confirmation
  prompt (skippable with `--yes`, required for non-TTY use); `tunnel`
  opens `ssh -L 11901:localhost:11901 -L 7575:localhost:7575 -L
  8082:localhost:8082` against the provisioned VM, defaulting the
  host / user / identity to the terraform outputs `elastic_ip` /
  `ssh_command` / `ssh_key_path` and accepting `--host` / `--user` /
  `--identity` overrides. Replaces the bespoke `tunnel.sh` scripts
  `murmures` and `terraform-provider-canton` CI carry today.
  Internals: `cli/internal/terraform` wraps the terraform binary
  through an injectable Runner (so tests don't shell out) and
  decodes `terraform output -json` into a typed `Outputs` struct;
  `cli/internal/tunnel` builds the ssh argument vector with the
  splice port set as `DefaultPorts` and exposes a `Client.Open` that
  shells out to `ssh` and propagates `ctx.Done()` for clean Ctrl-C
  teardown.
- `csharp/Peaceful.Canton.Localnet.Testing` — DAR upload, party allocation,
  and user binding (issue #9). Three new sibling deep modules of
  `JsonLedgerAdminClient`, wired into `LocalnetFixture` via the existing
  DI / `IHttpClientFactory` graph and exposed as convenience pass-throughs:
  - `DarUploader.UploadAsync` / `UploadManyAsync` — `POST /v2/packages`
    with raw DAR bytes (`application/octet-stream`). Idempotent: a
    `400 KNOWN_PACKAGE_VERSION` response is treated as success
    (`DarUploadOutcome.AlreadyKnown`). `UploadManyAsync` sequences
    uploads in input order and stops at the first genuine failure.
  - `PartyAllocator.AllocateAsync` — `POST /v2/parties` with hint
    `<consumer-prefix>-<instance-suffix>`. The instance suffix is a
    12-hex cryptographic random generated once per allocator instance
    so concurrent test runs and reruns against a long-lived stack
    don't collide. Returns `AllocatedParty(PartyId, PartyIdHint, IsLocal)`.
  - `UserBuilder.CreateAsync` — `POST /v2/users` followed by
    `POST /v2/users/{id}/rights` to grant `CanActAs`/`CanReadAs`. The
    rights call is skipped when both lists are empty.
  - `LocalnetFixture.UploadDarAsync` / `UploadDarsAsync` /
    `AllocatePartyAsync` / `CreateUserAsync` — convenience pass-throughs
    matching the README's 30-line example.
  - `csharp/tests/.../TestData/splice-util-0.1.0.dar` — vendored from
    the upstream `hyperledger-labs/splice` 0.6.2 image (already credited
    in `NOTICE`) so the smoke test has a real DAR to upload without a
    local daml SDK. The DAR is committed and copied to the test output
    directory; the smoke test self-skips when the localnet env vars are
    absent, matching the existing pattern.
- `csharp/Peaceful.Canton.Localnet.Testing` — xUnit fixture v0 (issue #7).
  Four sub-modules:
  - `EndpointDiscovery` — resolves the JSON Ledger API base URL, Keycloak
    token endpoint, audience, and client credentials from
    `CANTON_LOCALNET_*` env vars (defaults match the compose stack in
    `compose/modules/localnet/env/common.env`).
  - `OAuth2TokenProvider` — `client_credentials` grant with thread-safe
    in-memory cache, configurable expiry leeway (default 30 s), and
    refresh on expiry. Surfaces token-endpoint errors as
    `OAuth2TokenException`.
  - `JsonLedgerAdminClient` — `HttpClient` + `System.Text.Json` wrapper.
    v0 implements `GET /v2/parties/participant-id` only; DAR upload,
    party allocation, and user binding land in #9.
  - `LocalnetFixture` — `IAsyncDisposable` surface that composes the
    above via `Microsoft.Extensions.DependencyInjection` and exposes
    `GetParticipantIdAsync`.
  Built and packed as `Peaceful.Canton.Localnet.Testing.<version>.nupkg`
  (not yet published).
- `.github/workflows/csharp.yml` — thin caller for the reusable
  `peacefulstudio/github-actions/.github/workflows/csharp-ci.yaml`
  workflow. Passes `working-directory: csharp`, the
  ubuntu/macos/windows OS matrix, the `Category!=Integration` test
  filter, and `pack: true` so the fixture nupkg is built and uploaded
  as a workflow artifact. Replaces the legacy in-tree
  `csharp-ci.yaml` (single-OS, no integration filter, included a
  test-discovery bug that ran `dotnet test` against the production
  assembly).
- `terraform/` — EC2 spot instance + security group + Elastic IP
  configuration migrated from `peacefulstudio/murmures`
  `infra/terraform/`. Backend points at the shared
  `cicd-playground-tfstate` bucket at a new state key
  `canton-localnet/vm/terraform.tfstate`; the murmures state at
  `murmures/localnet/terraform.tfstate` is left intact. Resource names
  / SG name / key-pair name preserve the `murmures-localnet` prefix so
  `terraform import` produces a clean plan; the `Project` tag is the
  only intentional value change (`murmures` → `canton-localnet`), used
  to scope the CI IAM policy. Variable names and output names
  (`instance_id`, `elastic_ip`, `ssh_command`, `ssh_key_path`,
  `region`, `ami_id`) match murmures so consumer tunnel scripts work
  unchanged after switching their output source.
- `terraform/iam-policy.json` — least-privilege IAM policy for the
  canton-localnet GitHub Actions OIDC role: read/write the new S3 state
  key, EC2 / SG / EIP write actions scoped by
  `aws:ResourceTag/Project = canton-localnet`, `ec2:Describe*` read-only
  (no resource-level conditions available for those).
- `terraform/README.md` — step-by-step state-migration runbook for the
  maintainer-only one-time import (`terraform import` commands per
  resource, expected `terraform plan` outcome, rollback plan).
- `terraform-ci.yaml` workflow hardened: SHA-pinned
  `actions/checkout@v6.0.2`, `hashicorp/setup-terraform@v3.1.2`, and
  `marocchino/sticky-pull-request-comment@v3.0.4`. Runs
  `terraform fmt -check -recursive` + `terraform validate` (via
  `init -backend=false`) on PRs touching `terraform/**`. No
  `terraform plan` in CI until the IAM grant in the migration runbook
  lands.
- This change is **human-in-the-loop**: the terraform code is in;
  the AWS state migration and CI IAM grant are a maintainer checklist
  in the merging PR body, not executed by the agent.
- `cli/` — `canton-localnet` Go binary (module
  `github.com/peacefulstudio/canton-localnet/cli`) with three
  subcommands: `up`, `down`, `wait-ready`. `up`/`down` wrap the same
  `docker compose -f ...` invocation as the top-level Makefile (same
  modules, env-files, profiles, and `OBS` / `PQS` / `--auth` /
  `--no-resource-limits` toggles); `wait-ready` polls the JSON Ledger
  API readiness endpoint (default `http://localhost:3975/readyz`) with
  a configurable `--timeout` (default 5 minutes), `--interval`, and
  `--request-timeout`. Built with `spf13/cobra`. Internals:
  `cli/internal/compose` (compose plan assembly, table-tested across
  OBS / PQS / RES / auth toggles), `cli/internal/health` (httptest-tested
  readiness polling with success + timeout + context-cancel coverage),
  `cli/internal/repo` (walks up from `cwd` to find the
  `compose/modules` root, overridable via `--repo-root`).
- `.github/workflows/cli.yml` — matrix `go build ./cli/...` +
  `go test -race ./cli/...` on `ubuntu-latest` and `macos-latest`, plus
  a separate ubuntu-only smoke job that runs `canton-localnet up`,
  `canton-localnet wait-ready --timeout 10m`, then
  `canton-localnet down`. Third-party actions are SHA-pinned and every
  `run:` block sets `-euo pipefail`.
- `LICENSE` (Apache-2.0) and `NOTICE` files.
- Community files: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`,
  `.github/ISSUE_TEMPLATE/` (bug report, feature request, config), and
  `.github/PULL_REQUEST_TEMPLATE.md`.
- Apache-2.0 badge, project stewardship paragraph, Contributing block,
  and License section in `README.md`.
- `compose/` — Canton LocalNet stack vendored from
  `hyperledger-labs/splice` 0.6.2 (SHA pinned in `compose/splice.sha`).
  Includes the `localnet`, `keycloak`, `pqs`, and `splice-onboarding`
  modules. Re-vendoring is automated via `make vendor` and
  `compose/scripts/vendor.sh` driven by `compose/links.csv`.
- Top-level `Makefile` with `up` / `down` / `wait-ready` / `vendor`
  targets. OAuth2 is the default auth mode; shared-secret mode is
  available via `make up AUTH_MODE=secret` as an undocumented escape
  hatch and is not CI-tested.
- `compose-ci.yaml` workflow: smoke-tests `make up && make wait-ready`
  on `ubuntu-latest` against the OAuth2 stack on every PR touching
  `compose/**` or the Makefile.
- `go/fixture/` — Go module
  (`github.com/peacefulstudio/canton-localnet/go/fixture`) providing
  the Go-side integration-test harness paired with the C# fixture.
  Exports `EndpointDiscovery` (env-driven URL resolution),
  `OAuth2TokenProvider` (client_credentials grant with cached
  refresh, 30 s pre-expiry skew, coalesced concurrent fetches),
  `JsonLedgerAdminClient` (v0 implements `GET /v2/parties/participant-id`),
  and `Fixture` (composes the above behind `Setup` / `Teardown`).
  Stdlib-only.
- `go/fixture/dar_uploader.go`, `party_allocator.go`,
  `user_builder.go` — three deep modules added on top of the v0 Go
  fixture (issue #10, Go-side parity with #9). `DarUploader` posts
  raw DAR bytes to `POST /v2/packages` and treats an HTTP 400 whose
  body contains `KNOWN_PACKAGE_VERSION` as success, so repeated
  uploads of the same DAR (possibly with a different hash from a
  non-deterministic build) are idempotent. `UploadAll` runs DAR paths
  sequentially and stops at the first genuine failure.
  `PartyAllocator` posts to `POST /v2/parties` with hints of the form
  `<consumer-prefix>-<instance-suffix>` where the suffix is a random
  16-character hex string generated once per allocator instance.
  `UserBuilder` posts `{user, rights}` to `POST /v2/users` and then,
  if any `ActAs`/`ReadAs` parties are supplied, grants them via
  `POST /v2/users/{id}/rights` using the `CanActAs` / `CanReadAs`
  right kinds. All three are wired onto `*Fixture` as the convenience
  methods `UploadDar`, `UploadDars`, `AllocateParty`, and
  `CreateUser`. Stdlib-only.
- `go/fixture/smoke_integration_test.go` — extended with
  `TestSmoke_AllocatePartyUploadDarBuildUser`, which exercises party
  allocation, DAR upload (twice, to assert the
  `KNOWN_PACKAGE_VERSION` idempotency path against the live ledger),
  and user creation end-to-end. The DAR portion is gated on
  `CANTON_LOCALNET_TEST_DAR_PATH`; when unset the rest of the test
  still runs.
- `.github/workflows/go-fixture.yaml` — integration job now extracts
  one DAR from the running `splice-onboarding` container (`docker cp`
  from `/canton/dars/*.dar`) before the smoke test and exposes the
  path via `CANTON_LOCALNET_TEST_DAR_PATH`, so the DAR-upload portion
  of the new integration test runs against a real DAR without
  committing one to the repo.
- `go-fixture.yaml` workflow: runs `go vet` and `go test -race` for
  `go/fixture/` on ubuntu-latest and macos-latest, plus an
  integration smoke job on ubuntu-latest that boots the compose
  stack and runs `go test -tags integration` against it.
- README quickstart documenting the `make up` / `make wait-ready` /
  `make down` loop and the JSON Ledger API ports per profile.
- Third-party attribution paragraph in `NOTICE` crediting
  `hyperledger-labs/splice` (and its earlier home,
  `digital-asset/cn-quickstart`) as the upstream source for the
  vendored compose modules. Upstream Digital Asset copyright headers
  on each file are preserved verbatim.

### Changed

- Refactored Go fixture HTTP path into a shared helper (#29). The
  POST + bearer-auth + body-capture + status-check pattern duplicated
  across `DarUploader`, `PartyAllocator`, `UserBuilder` (and the
  pre-existing `JsonLedgerAdminClient`) is now one unexported
  `doRequest` in `go/fixture/httputil.go`, and the four
  per-constructor `strings.TrimRight(baseURL, "/")` calls collapse to
  a single `normalizeBaseURL`. The DAR `KNOWN_PACKAGE_VERSION`
  400-as-success shortcut is preserved as a per-call
  `treat400AsSuccess` hook. No public API change.
- Aligned C# smoke-test DAR strategy with Go (issue #30, follow-up to
  PRs #26 and #28): the C# `LocalnetFixture` upload smoke now reads the
  DAR path from the `CANTON_LOCALNET_TEST_DAR_PATH` env var (matching
  the Go side) and self-skips with `Assert.Skip` when the var is unset
  or points at a missing file. `.github/workflows/csharp.yml` grew a
  sibling `integration` job that boots the compose stack, extracts a
  DAR from the running splice-onboarding container via `docker cp`
  (same shell as the Go workflow's extraction step), and runs the
  `Category=Integration` filter against it; compose diagnostics on
  failure upload as `compose-diagnostics-csharp` so they don't collide
  with the Go job's artifact. The previously vendored
  `csharp/tests/.../TestData/splice-util-0.1.0.dar` (226 KB) is removed
  from the repo along with its NOTICE entry and the `<None Include>`
  copy block in the test csproj — the DAR now comes "for free" with
  whatever splice version compose is running, so a splice bump no
  longer requires a manual DAR re-vendor.
- Licensing established as Apache-2.0. Every new source file must carry
  the two-line SPDX header (`Copyright (c) YYYY Peaceful Studio OÜ` +
  `SPDX-License-Identifier: Apache-2.0`) regardless of language; for
  future .NET projects in this repo, the central `<Copyright>` tag in
  `Directory.Build.props` is retained alongside the per-file headers
  (it drives the assembly attribute).
- Hardened CI workflows (`automerge.yaml`, `claude.yaml`):
  SHA-pinned third-party action `dependabot/fetch-metadata`, added
  `set -euo pipefail` and fork-PR-token guards on write-API steps.
