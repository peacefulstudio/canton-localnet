# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.5-2] - 2026-09-09

Same Splice pin as `0.7.5-1`. This release gives shared-stack consumers a
way to take user rights without silting the participant: a leased grant that
hands them back on dispose, a strict revoke inverse, and a CLI
`rights list` / `rights prune` pair for cleaning up leaks a killed suite
left behind.

### Added

- An inverse for user-rights grants on `Peaceful.Canton.Localnet.Testing`
  (#163). `UserBuilder.RevokeRightsAsync` issues the `PATCH
  /v2/users/{id}/rights` that undoes `GrantRightsAsync`. It is the strict
  inverse: it throws unless the participant reports every requested right as
  newly revoked, so a right that was already gone counts as a shortfall and
  calling it twice for the same parties throws the second time. A silent
  partial revoke leaves rights behind, which is precisely the failure this
  closes.

  `UserBuilder.GrantRightsLeaseAsync` grants and returns a
  `UserRightsLease` — a new public `IAsyncDisposable` — that hands the
  rights back on dispose. It owns **only** the rights the participant
  reported as `newlyGrantedRights`, never the full requested set: a right
  that was already on the user belongs to whoever granted it first, and
  revoking it here would break them. A lease that newly granted nothing
  therefore owns nothing, reports an empty `Rights`, and disposes without a
  request; `Rights`, `ActAs` and `ReadAs` say what it holds, so an
  overlapping second lease is observable rather than a silent no-op. The
  rights it holds are kept as the participant's own JSON and echoed back
  verbatim on revoke, so a right variant this package does not model round
  trips instead of being flattened into a shape the participant rejects.

  Disposal revokes on `CancellationToken.None`, so a run cancelled
  mid-flight still returns its rights, and the grant itself is issued the
  same way — the caller's token governs the run-up to it and nothing after,
  because a cancellation landing between the participant committing the
  rights and the lease being returned would strand them. A revoke that
  succeeded is never repeated. One that fails throws — an invisible leak is
  worse than a visible failure — and leaves the lease disposable again,
  narrowed to the rights the participant did not confirm, so the hand-back
  can be retried; a retry that finds the rights already gone from the user
  settles rather than looping. `UserRightsGrantedWithoutLeaseException` (a
  `JsonLedgerApiException`) covers the one case where rights are committed
  and no lease can be built: it carries the requested parties, and names the
  call that hands them back.

  Both reach the fixtures as `GrantUserRightsLeaseAsync` and
  `RevokeUserRightsAsync` on `LocalnetFixture` and `ValidatorFixture`.

  Until now `GrantUserRightsAsync` had no inverse anywhere on the package's
  public surface, so every consumer that granted rights had to hand-roll its
  own revoke-on-teardown; `canton-ledger-api-csharp` did exactly
  that. A Canton participant caps a user at 1000 rights and parties are
  never deletable, so a long-lived shared LocalNet silts up until command
  submission fails with `TOO_MANY_USER_RIGHTS`. `GrantUserRightsAsync` keeps
  its signature, and its behaviour except for the deduplication noted under
  *Changed* — a caller who repeats a party in one list now puts one right on
  the wire instead of two. Its XML doc points at the leased variant as the
  preferred way to take rights on a shared stack, and
  `docs/public/integration-testing.md` gains a *User rights on a shared
  stack* section.

- An integration test pinning the JSON Ledger API's encoding of empty rights
  lists (#163), run by the `integration (compose stack)` lane against a live
  participant. The OpenAPI spec marks `newlyGrantedRights`,
  `newlyRevokedRights` and `rights` as optional with no `required` array, and
  the lease's design turns on the difference between an absent field and an
  empty one, so the test asserts on the raw response bodies for a grant that
  was already held, a revoke of a right already gone, and a user holding no
  rights. Its failure message carries the participant version, so a Splice
  repin that changes the encoding reports itself.

- `canton-localnet rights list --slot <s>` and `canton-localnet rights
  prune --slot <s>`, with `make list-rights` / `make prune-rights`
  wrapping them. A Canton participant caps a user at 1000 rights and
  never deletes a party, so every `CanActAs` grant an integration suite
  makes on an ephemeral party is permanent; a run killed before its
  teardown leaks its grants, and a long-lived shared LocalNet silts up
  until further grants fail with `TOO_MANY_USER_RIGHTS`. `rights list`
  is the audit read — it splits the slot's validator user's rights into
  what a prune would preserve and what it would revoke, collapsing
  ephemeral party families such as `grpc-writer-parity-<hex>::…` onto a
  single counted row, or naming every party with `--full`. `rights
  prune` is the sweep; its contract and rails are stated in `rights
  prune --help` and restated in the plan it prints on every run. Both
  reuse the existing `auth token` minting, so the bearer and the
  Keycloak client secret never reach output, a log line, an error, or a
  process argument.

  `prune` writes nothing without `--yes` or an answered interactive
  prompt. `--dry-run` plans and runs every check without writing, and is
  mutually exclusive with `--yes`. The preserved party is read from the
  participant as the target user's `primaryParty` — it is not derivable
  from the slot name, since `sv-validator-1` operates as the founded
  party `sv::<ns>` — and `--preserve-party <party>` overrides that
  lookup, requiring a full party id containing `::` that the user still
  holds an act-as right on. `--max-revoke` (default 1000, the
  participant's own per-user cap) bounds a runaway revoke list.

  `prune` exits `2` when the sweep was not authorised, because the
  prompt was declined or `--yes` was withheld from a non-interactive
  run; `0` when there was nothing to revoke, the revoke succeeded, or
  `--dry-run` planned a run that passes every check; `1` for everything
  else, including an invocation rejected before any ledger call. Only
  `2` guarantees nothing changed — a partial revoke exits `1` with
  rights already gone — so a `1` calls for a `rights list` before
  assuming the state. The distinct `2` keeps a deliberate "no", and a CI
  job that forgot `--yes`, from looking like a completed sweep.

- `slot.Endpoints.ValidatorUserID`, resolved as
  `CANTON_LOCALNET_<SLOT>_USER_ID` > `AUTH_<SLOT>_VALIDATOR_USER_ID` in
  `compose/modules/keycloak/env/<slot>/on/oauth2.env` > a built-in
  default, falling back to the HS256 token subject for `sv-validator-1`.
  A test asserts the built-in defaults still match the compose env tree.

### Changed

- Duplicate parties within a single `actAs` or `readAs` list are collapsed
  before the request is sent (#163). Concatenating two party lists used to
  send the same right twice, which the participant reports as one — and, with
  the strict revoke above, would have thrown while claiming a right was still
  granted. The same party in both `actAs` and `readAs` is unaffected: those
  are two distinct rights.

- `JsonLedgerApiException` is no longer `sealed` (#163), so
  `UserRightsGrantedWithoutLeaseException` can extend it and existing
  `catch (JsonLedgerApiException)` clauses keep catching it. That is the
  point of subclassing — a teardown handler written before this signal
  existed still runs — and it is also the hazard: such a handler that
  swallows rather than logs will discard the one report that rights are
  stranded on the participant. Deriving from `Exception` instead would make
  those handlers miss it entirely, which is worse, so check any `catch
  (JsonLedgerApiException)` you own on a rights path. Unsealing is one-way:
  re-sealing later would be a binary break. xUnit's `Assert.Throws<T>` and
  `ThrowsAsync<T>` match the exact type rather than the hierarchy, so a suite
  that asserts the base type around a call that can now throw the derived one
  needs `ThrowsAny<JsonLedgerApiException>`. No shipped call path throws the
  subclass today — `UserBuilder.GrantRightsLeaseAsync` is new in this release
  — but every consumer of this package is an xUnit suite, so that is how the
  subclass will be met.

### Fixed

- Clear `UserRightsLease.Rights` / `ActAs` / `ReadAs` on a successful dispose,
  and count the partial-revoke exception from the narrowed set the retry's
  `GET` proved (#167). A lease that had handed everything back used to keep
  naming the rights it no longer held, so a consumer asking "did anything
  leak?" got a false positive on the one path where nothing did; the
  exception message could also over-count rights the `GET` had already
  proved gone.

## [0.7.5-1] - 2026-09-02

Vendored Splice moves 0.7.3 → 0.7.5. Drop-in from `0.7.3-1`: no breaking
change, no volume wipe, and no config you hold needs editing.

The only behaviour change reaches the `--multi-sync` lane, whose bootstrap
now follows upstream's hardened `EnableMultiSynchronizer` flow; that lane
also gains gating CI coverage for the first time. Single-synchronizer
consumers see an image-tag bump and nothing else.

### Changed

- Upgrade the vendored Splice / Canton LocalNet from 0.7.3 to 0.7.5
  (#159), pinned to upstream `hyperledger-labs/splice`
  `858a7347aaeb958657b76b0957186102b74e3bd6` in `compose/splice.sha` and
  `compose/links.csv`; `SPLICE_VERSION=0.7.5` in `compose/.env.defaults`
  drives the `canton`, `splice-app` and web-ui image tags. Of the 93
  upstream commits between the two tags, exactly one reaches
  `cluster/compose/localnet`: `49c770819` *"fix: added missing feature
  flag for multisync (#6809)"*, which hardens the multi-synchronizer
  bootstrap in `conf/console/app-synchronizer.sc`. No volume wipe is
  needed: the `POSTGRES_VERSION=18` and `NGINX_VERSION=1.30.0` pins, the
  postgres `/var/lib/postgresql` mount path, the PQS `SCRIBE_VERSION`
  pin, the 5-validator topology and the per-slot `env/` wiring are all
  untouched. `make config` renders 18 services, with every Splice image
  at the 0.7.5 tag. The C# package version moves to `0.7.5-1`.

### Added

- Gating CI coverage for the multi-synchronizer bootstrap (#159). The
  `integration-tests` matrix gains a `multi-sync` scenario that brings the
  stack up with `canton-localnet up --multi-sync` and runs
  `tests/acceptance/multi-sync.sh`, which asserts that
  `multi-sync-startup` exited 0 (its final `retry_until_true` re-reads the
  trust certificates, so a zero exit means `EnableMultiSynchronizer`
  actually became effective), that its logs carry no console failure, that
  the `a`/`b`/`d` validators each report 2 connected synchronizers over the
  JSON Ledger API, and — as a negative control — that `c-validator-1` is
  healthy on only 1. The scenario runs on any `compose/**`, `cli/**` or
  `Makefile` change and on every push to `dev`. Until now
  `conf/console/app-synchronizer.sc` was rewritten on each Splice bump but
  executed by nothing in this repo; its only coverage was a weekly,
  non-gating lane in a consumer repo.

### Fixed

- Multi-synchronizer bootstrap (`compose/modules/localnet/conf/console/app-synchronizer.sc`)
  now adopts upstream's hardened `EnableMultiSynchronizer` flow instead of
  the equivalent block this repo had hand-rolled ahead of upstream, applied
  to the `a`/`b`/`d` validator slots. Two real behaviour fixes come with it,
  plus one piece of hardening. The console now waits until
  each participant is connected to the **global** synchronizer before
  proposing, closing a race in which the flag *could* land on
  `app-synchronizer` only; and it **appends** to the participant's existing
  feature flags instead of replacing them, so no unrelated flag is dropped.
  It additionally re-checks that the flag became effective before the
  bootstrap container exits, and guards the proposal so a re-run is a
  no-op — hardening rather than a fix, since the one-shot
  `multi-sync-startup` container never ran the old block twice. Only the
  `MULTI_SYNC=true` lane is affected.

## [0.7.3-1] - 2026-08-21

Rolls up two vendored Splice bumps — 0.7.0 → 0.7.1 → 0.7.3. `0.7.1-1` was
never cut, so upgrading from `0.7.0-1` crosses both in one step.

Drop-in from `0.7.0-1`: no breaking change, no volume wipe, and no config
you hold needs editing. The Postgres 18 boundary was crossed back in
`0.7.0-1` and is not re-crossed here.

### Changed

- Upgrade the vendored Splice / Canton LocalNet from 0.7.0 to 0.7.1
  (#153), pinned to upstream `hyperledger-labs/splice`
  `c95e1ef5c939cef13cc344d7678f9d2ce2474c6a` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.7.1` in
  `compose/.env.defaults` drives the `canton`/`splice-app`/web-ui
  image tags. Upstream's only change to `cluster/compose/localnet`
  between the two tags is in `env/splice.env`: the traffic-topup
  target is now overridable as
  `TARGET_TRAFFIC_THROUGHPUT=${TARGET_TRAFFIC_THROUGHPUT:-20000}`
  instead of a hardcoded `20000`, so exporting
  `TARGET_TRAFFIC_THROUGHPUT` before `make up` now takes effect. The
  rendered default is unchanged at `20000`, and the three-way merge
  produced no conflicts. No volume wipe is needed: the
  `POSTGRES_VERSION=18` and `NGINX_VERSION=1.30.0` pins, the PQS
  `SCRIBE_VERSION` pin, the 5-validator topology and the per-slot
  `env/` wiring are all untouched. The C# package version is bumped
  to `0.7.1-1`.

- Upgrade the vendored Splice / Canton LocalNet from 0.7.1 to 0.7.3
  (#154), pinned to upstream `hyperledger-labs/splice`
  `0dd6b9263510007e831bcc3ca3a44a41aa862a75` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.7.3` in
  `compose/.env.defaults` drives the `canton`/`splice-app`/web-ui
  image tags. Upstream made no change at all to
  `cluster/compose/localnet` between the two tags — the subtree is
  byte-identical (`ef34a929dc1c47250f12dacf840b753b50100508`) — so
  this is a pure image-tag bump and no file under
  `compose/modules/localnet/` changed. No volume wipe is needed: the
  `POSTGRES_VERSION=18` and `NGINX_VERSION=1.30.0` pins, the PQS
  `SCRIBE_VERSION` pin, the 5-validator topology and the per-slot
  `env/` wiring are all untouched. The C# package version is bumped
  to `0.7.3-1`.

## [0.7.0-1] - 2026-08-13

First stable release since `0.6.5-2` (NuGet `0.6.5.2`, 2026-06-10).
Functionally identical to `0.7.0-1.preview.1` — no code changes; this
promotes that preview to a stable four-part NuGet version, `0.7.0.1`.

Upgrading from `0.6.5.2` crosses six vendored Splice bumps (0.6.9,
0.6.10, 0.6.11, 0.6.13, 0.6.14, 0.7.0) and the three breaking changes
below. Per-release detail is in the `[0.6.9-1.preview.1]` through
`[0.7.0-1.preview.1]` sections.

### Changed — BREAKING

- **Wipe every existing stack before the first `make up` on this
  version.** The postgres 17 → 18 bump and the domain-neutral party
  hints each invalidate existing volumes on their own; postgres or the
  validator will otherwise restart-loop. Run `make clean` (or
  `./canton-localnet down --volumes`) first.
- **`make up` toggles take `true`/`false` instead of `on`/`off`.**
  Callers passing `RES=on` / `PQS=on` / `OBS=on` (or `=off`) must
  switch to `=true` / `=false`.
- **Validator party hints default to the slot name** —
  `a-validator-1`, `b-validator-1`, `c-validator-1`, `d-validator-1` —
  instead of `localnet-validator-1`, `alice-validator-1`,
  `bob-validator-1` and `danielle-validator-1`. Tests or tooling
  asserting on the old hints must be updated.

## [0.7.0-1.preview.1] - 2026-08-05

### Changed

- **BREAKING (postgres 18):** the postgres image pin moves from 17 to
  18 (resolving to 18.4) — `POSTGRES_VERSION` in
  `compose/modules/localnet/compose.env`. PostgreSQL 18 cannot read a
  version-17 data directory, so **any existing stack — local or the
  shared Hetzner VM — must be wiped before the next `make up`**: run
  `make clean` (or `./canton-localnet down --volumes`) first.
  Skipping the wipe leaves `postgres` unable to start against the old
  data directory, and because every other service depends on it the
  whole stack never comes up. The bump also forces a mount-path
  change: the
  `postgres` volume now mounts at `/var/lib/postgresql` instead of
  `/var/lib/postgresql/data` in
  `compose/modules/localnet/compose.yaml`, because postgres:18 stores
  data in major-version-specific subdirectories and refuses to start
  against the old path (docker-library/postgres#1259). That one bites
  on a *fresh* volume, not merely a stale one.
- Upgrade the vendored Splice / Canton LocalNet from 0.6.14 to 0.7.0
  (#146), pinned to upstream `hyperledger-labs/splice`
  `a9076eb91f87a9bd9315d2f9e122d6350bdc9d4c` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.7.0` in
  `compose/.env.defaults` drives the `canton`/`splice-app`/web-ui
  image tags. Upstream made no changes to `cluster/compose/localnet`
  between the two tags — the BASE and THEIRS trees are byte-identical,
  so the three-way merge left every shared file "upstream unchanged",
  making the vendored-tree upgrade a pure version-pin bump; that is
  the third consecutive release with an unchanged vendored tree. Both
  of upstream 0.7.0's own breaking changes are no-ops here: neither
  the Scan `/transactions` endpoint nor `TransferCommand` is
  referenced by the compose stack or by any fixture in this repo. The
  `NGINX_VERSION=1.30.0` pin, the 5-validator topology and the
  per-slot `env/` wiring are untouched. The C# package version is
  bumped to `0.7.0-1`.
- Move the PQS `SCRIBE_VERSION` default from 0.6.14 to 3.5.7 in
  `compose/modules/pqs/compose.env`. scribe versions independently of
  Splice — there is no scribe 0.7.0 — so this is a deliberate move
  off the stale legacy 0.6.x line onto the current line, which tracks
  Canton 3.5.x, and not a lockstep bump with the Splice pin. The
  3.5.7 image moves its `WorkingDir` from `/daml3.4/` to `/` while
  keeping a relative jar path in its entrypoint, so the four
  `working_dir: /daml3.4` lines are removed from
  `compose/modules/pqs/compose.yaml`; retaining them fails with
  `Unable to access jarfile scribe.jar`. PQS is opt-in
  (`make up PQS=true`) and off in CI and in the shared VM's default
  profile, so pass `SCRIBE_VERSION=0.6.14` to stay on the previous
  image.

## [0.6.14-1.preview.1] - 2026-07-28

### Added

- New documentation guides under `docs/public/`:
  `topologies.md` (the flexible-topology narrative — one
  super-validator plus N validators, the five named slots, the
  optional observability / PQS / multi-synchronizer layers, and a
  scenario-to-config matrix) and `integration-testing.md` (a
  consumer HOWTO for attaching the C# and Go test fixtures to a
  running stack), both indexed from a new `docs/public/README.md`
  landing page.
- Internal `localnet-docs-release-update` skill that runs a
  doc-freshness sweep at release time, flagging stale version
  strings and drifted capability/slot references before publish.

### Changed

- **BREAKING (party hints):** party hints now default to the slot
  name on every validator slot (`a-validator-1`, `b-validator-1`,
  `c-validator-1`, `d-validator-1`) instead of the
  consumer-flavoured `localnet-validator-1`, `alice-validator-1`,
  `bob-validator-1` and `danielle-validator-1`. This closes a
  divergence where `make up` and `./canton-localnet up` bootstrapped
  the `a` slot with different hints, leaving each path's postgres
  volume unusable by the other: the container gets stuck restarting
  (climbing `RestartCount`) with `does not match configured hint` in
  `docker logs`, not a Docker-reported `unhealthy` status. It also
  makes `partyHint:` in `canton-localnet.yaml` effective on the `b`,
  `c` and `d` slots, where hardcoded module env values had silently
  overridden it. **Existing volumes are invalidated on both paths**
  — the recorded hint no longer matches the configured one. Run
  `./canton-localnet down --volumes` (or `make clean`) before
  upgrading. `partyHint:` renames the validator itself; the users
  hosted on a validator are named separately via that slot's
  `parties:` list — see `docs/public/canton-localnet-yaml-schema.md`.
  (#137)
- Upgrade the vendored Splice / Canton LocalNet from 0.6.13 to 0.6.14
  (#135), pinned to upstream `hyperledger-labs/splice`
  `398919a5b13479877fd61587003ba7a4ba00091b` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.6.14` in
  `compose/.env.defaults` drives the `canton`/`splice-app`/web-ui image
  tags. Upstream made no changes to `cluster/compose/localnet` between
  the two tags — the BASE and THEIRS trees are byte-identical, so the
  three-way merge left every shared file "upstream unchanged", making
  the vendored-tree upgrade a pure version-pin bump. The
  `POSTGRES_VERSION=17` / `NGINX_VERSION=1.30.0` pins and the per-slot
  `env/` wiring are untouched. The C# package version is bumped to
  `0.6.14-1`.
- Bump the PQS `SCRIBE_VERSION` default from 0.6.13 to 0.6.14 in
  `compose/modules/pqs/compose.env`, aligning it with the Splice pin
  (it had been pinned independently at 0.6.13 since 0.6.9). PQS is
  opt-in (`make up PQS=true`) and off in CI, so pass
  `SCRIBE_VERSION=0.6.13` to stay on the previous image.
- Repositioned the documentation to lead with capabilities, use
  cases, and the flexible topology rather than a bare quickstart:
  the root `README.md` now opens with a capabilities matrix and the
  supported use cases, linking out to the new topology and
  integration-testing guides. Also refreshed stale version and
  topology references across the docs and skills.

### Removed

- Stop publishing `Peaceful.Canton.Localnet.Testing` to the GitHub
  Packages NuGet feed on tag push — nuget.org (via `publish.yaml`,
  triggered on GitHub Release publish) is now the sole NuGet
  distribution channel. The GitHub Packages push had been failing
  on an org-level billing/storage quota, and no downstream consumer
  restored from that feed; all previously published versions were
  removed from GitHub Packages.

### Fixed

- Multi-synchronizer bring-up (`MULTI_SYNC=true make up`) no longer
  fails at the `multi-sync-startup` container. `app-synchronizer.sc`
  referenced `ParticipantTopologyFeatureFlag` without importing it,
  so the Canton console rejected the whole script with
  `not found: value ParticipantTopologyFeatureFlag` /
  `Compilation Failed` before any statement ran — meaning the
  synchronizer bootstrap itself never executed and the multi-sync
  profile could not start at all. Added the missing
  `com.digitalasset.canton.topology.transaction.SynchronizerTrustCertificate.ParticipantTopologyFeatureFlag`
  import. The enum value `EnableMultiSynchronizer` was already
  correct; only the enclosing object was out of scope.

## [0.6.13-1.preview.1] - 2026-07-21

### Changed

- Upgrade the vendored Splice / Canton LocalNet from 0.6.11 to 0.6.13
  (#128), pinned to upstream `hyperledger-labs/splice`
  `52ccdd6841f3eda11fe86313e8cdf9540efedfb7` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.6.13` in
  `compose/.env.defaults` drives the `canton`/`splice-app` image tags.
  Upstream made no changes to `cluster/compose/localnet` between the two
  tags (0.6.12 skipped as an intermediate step) — the BASE and THEIRS
  trees are byte-identical, so the three-way merge left every shared file
  "upstream unchanged", making this a pure version-pin bump. The
  `POSTGRES_VERSION=17` / `NGINX_VERSION=1.30.0` pins and the per-slot
  `env/` wiring are untouched. C# package version bumped to `0.6.13-1`.

## [0.6.11-1.preview.1] - 2026-07-07

### Added

- Multi-sync bring-up (`MULTI_SYNC=true`) now proposes the Canton
  `EnableMultiSynchronizer` participant feature flag for the `a`/`b`/`d`
  validators on every synchronizer they're connected to, so downstream
  reassignment conformance tests run instead of skipping on the
  feature-flag-off guard. Single-sync bring-up is unaffected — the bootstrap
  script this lives in only ever runs under `--multi-sync`. (#121)

### Changed

- Upgrade the vendored Splice / Canton LocalNet from 0.6.10 to 0.6.11
  (#122), pinned to upstream `hyperledger-labs/splice`
  `fd93f86ac42ce3a08985dcd0baae530b4f235f60` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.6.11` in
  `compose/.env.defaults` drives the `canton`/`splice-app` image tags.
  Upstream made no changes to `cluster/compose/localnet` between the two
  tags — the BASE and THEIRS trees are byte-identical, so the three-way
  merge left every shared file "upstream unchanged" — making this a pure
  version-pin bump. The `POSTGRES_VERSION=17` / `NGINX_VERSION=1.30.0`
  pins and the per-slot `env/` wiring are untouched. C# package version
  bumped to `0.6.11-1`.

### Fixed

- The baked-in splice-onboarding `upload_dar` helper now discovers the
  connected synchronizers via `GET /v2/state/connected-synchronizers` and vets
  each DAR per synchronizer (`POST /v2/packages?synchronizerId=<id>`), instead
  of a bare upload that Canton could no longer autodetect once the
  multi-synchronizer profile connects a validator to two synchronizers
  (`PACKAGE_SERVICE_CANNOT_AUTODETECT_SYNCHRONIZER`). Synchronizer ids are
  discovered at run time because they are not stable across a localnet down/up,
  and re-uploads stay idempotent (`KNOWN_PACKAGE_VERSION` is treated as
  success). (#118)
- Pin `modules.obs`/`modules.pqs` off via a repo-root `canton-localnet.yaml` so
  bare `canton-localnet up` (CI's invocation) matches `make up` parity instead
  of falling through to `yamlconfig.Defaults()` (obs+pqs on), which
  overcommitted the CI runner and caused intermittent
  `integration (compose stack)` failures. (#124)

## [0.6.10-1.preview.1] - 2026-07-03

### Added

- Multi-synchronizer profile: `canton-localnet up --multi-sync` (and `MULTI_SYNC=true make up`) brings up Splice's `app-synchronizer`; `a`/`b`/`d` validators connect to both synchronizers. `wait-ready --synchronizers 2` gates on the connection count. Fixtures (C# + Go) gain `GetConnectedSynchronizers`/`GetAppSynchronizerId` to discover the second synchronizer id. Local/CI only. (#115)

### Changed

- **BREAKING (Make toggles):** the `RES`, `PQS`, and `OBS` make
  variables now take `true` / `false` instead of `on` / `off`, for
  consistency with the new `MULTI_SYNC` toggle. Callers of `make up`
  passing `RES=on` / `PQS=on` / `OBS=on` (or `=off`) must switch to
  `=true` / `=false`. (#115)
- Upgrade the vendored Splice / Canton LocalNet from 0.6.9 to 0.6.10
  (#117), pinned to upstream `hyperledger-labs/splice`
  `63cfb340ce0f8f254386d2d5df58905d695de902` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.6.10` in
  `compose/.env.defaults` drives the `canton`/`splice-app` image tags.
  Merged as a three-way merge that preserves this repo's 5-validator
  topology, the `POSTGRES_VERSION=17` / `NGINX_VERSION=1.30.0` pins, and
  the per-slot `env/` wiring. One upstream functional change carried
  over: postgres now initializes with `--data-checksums`. The
  PQS `SCRIBE_VERSION` (0.6.13) is an independent pin and is left
  unchanged. C# package version bumped to `0.6.10-1`.

### Fixed

- The `c-validator-1` console failed to start: its `entrypoint.sh`
  never exported `C_VALIDATOR_1_VALIDATOR_USER_TOKEN`, which the
  console config already references, so the unresolved variable
  aborted bring-up with a substitution error. The token is now
  exported like the other slots'. (#115)
- The `sv-validator-1` console keyed its remote participant under `sv`
  in `app-auth.conf` while `app.conf` (and every other validator) used
  `sv-validator-1`, so console commands failed with
  `Key not found: admin-api/ledger-api`. Both files now agree on
  `sv-validator-1`. (#115)

## [0.6.9-1.preview.1] - 2026-06-24

Preview of `0.6.9-1` (NuGet `0.6.9.1-preview.1`, opt-in prerelease) — the
Splice 0.6.9 LocalNet upgrade and the all-five-validator default.

### Changed

- Default LocalNet topology now runs all five validators (`sv`, `a`, `b`,
  `c`, `d`). Previously `c-validator-1` defaulted off; its
  `C_VALIDATOR_1_PROFILE` default in `compose/modules/localnet/compose.env`
  flips from `off` to `on`, so `make up` (and the CLI `canton-localnet up`,
  which already enabled all five) now agree on a full-fidelity local
  default. The shared Hetzner VM deployment runs a reduced `sv + a + b` set
  — `terraform/hetzner/templates/cloud-init.yaml.tftpl` exports
  `C_VALIDATOR_1_PROFILE=off` and `D_VALIDATOR_1_PROFILE=off` before
  `make up`, since `c` and `d` are not exercised by CI integration tests and
  dropping them saves resources on the billing box.
- Bump the `go-ci.yaml` reusable workflow pin from `@v1` to `@v2`. The `@v1`
  reusable carried a broken `sudo chown` coverage step that failed on
  self-hosted Hetzner runners (`sudo: a password is required`), breaking Go CI;
  the fix shipped in `@v2` (peacefulstudio/github-actions#29). The go-ci input
  contract is unchanged between v1 and v2, so this is a non-breaking repoint.
- Upgrade the vendored Splice / Canton LocalNet from 0.6.5 to 0.6.9
  (#105), pinned to upstream `hyperledger-labs/splice`
  `bc6a3587e7ea94230ba0c36c638945282c52b304` in `compose/splice.sha`
  and `compose/links.csv`; `SPLICE_VERSION=0.6.9` in
  `compose/.env.defaults` drives the `canton`/`splice-app` image tags.
  Merged as a three-way merge that preserves this repo's 5-validator
  topology, the `POSTGRES_VERSION=17` / `NGINX_VERSION=1.30.0` pins, and
  the per-slot `env/` wiring. Two upstream functional changes carried
  over: the splice container health check switched from `curl -f` to
  `wget --no-verbose --tries=1 --spider`, and `domain-migration-id`
  (`${?MIGRATION_ID}`) was dropped from `conf/splice/app.conf`. The
  PQS `SCRIBE_VERSION` (0.6.13) is an independent pin and is left
  unchanged. C# package version bumped to `0.6.9-1`.

## [0.6.5-3.preview.1] - 2026-06-14

Preview of `0.6.5.3` (NuGet `0.6.5.3-preview.1`, opt-in prerelease) and
the first published build to carry the changes below — the `0.6.5-3`
final was drafted but never pushed to nuget.org.

### Added

- Embed the Peaceful Studio package icon and NuGet `PackageTags`
  (`canton`, `localnet`, `splice`, `daml`, `xunit`, …) so the package
  presents with branding and is discoverable on nuget.org.
- Preview release lane: `v<X.Y.Z>-<N>.<label>` tags map to NuGet
  prereleases (`v0.6.5-3.preview.1` → `0.6.5.3-preview.1`).

### Fixed

- `DarUploader.UploadAsync` transparently retries a transient
  `503 Service Unavailable` from `POST /v2/packages` with bounded
  exponential backoff (6 attempts, 1s base capped at 16s), tunable via
  the new optional `DarUploaderRetryOptions` parameter; `2xx` and the
  idempotent `400 KNOWN_PACKAGE_VERSION` outcome are unchanged, other
  failures (`401`/`403`) still fail fast.
- NuGet publish no longer fails at the test step — dropped the legacy
  VSTest `--filter` flag that Microsoft.Testing.Platform rejects.

## [0.6.5-3] - 2026-06-13

### Added

- Embed the Peaceful Studio package icon and NuGet `PackageTags`
  (`canton`, `localnet`, `splice`, `daml`, `xunit`, …) in
  `Peaceful.Canton.Localnet.Testing`, completing the
  `dotnet-extensions` packaging baseline so the package presents with
  branding and is discoverable on nuget.org (#98).

### Fixed

- `DarUploader.UploadAsync` now transparently retries a transient
  `503 Service Unavailable` from `POST /v2/packages` with bounded
  exponential backoff (6 attempts, 1s base delay capped at 16s),
  fixing intermittent integration-suite failures on slow CI runners
  where the package service is still warming up on the first ledger
  call (#96). Retry behaviour is tunable via the new optional
  `DarUploaderRetryOptions` constructor parameter; `2xx` success and
  the idempotent `400 KNOWN_PACKAGE_VERSION` outcome are unchanged, and
  all other failures (e.g. `401`/`403`) still fail fast with no retry.

## [0.6.5-2] - 2026-06-10

### Added

- Publish `Peaceful.Canton.Localnet.Testing` to nuget.org whenever a
  GitHub Release is published, as a stable four-part version mapped
  from the tag (`v0.6.5-1` → `0.6.5.1`) so plain `dotnet add package`
  resolves it without `--prerelease` (#88). GitHub Packages keeps the
  tag-verbatim prerelease version (`0.6.5-1`).

## [0.6.5-1] - 2026-06-06

### Added

- `canton-localnet auth token --slot {sv|a|b|c|d}` and
  `canton-localnet info --slot X [--json] [--offline]` CLI commands
  (#64). The `auth token` command mints a participant-admin bearer —
  OAuth2 `client_credentials` against Keycloak for a/b/c/d, self-signed
  HS256 against the LocalNet shared secret for sv. The `info` command
  prints the slot's ports, Keycloak realm, audience, token URLs (host
  and internal), and — when LocalNet is reachable — `participant_id`,
  `participant_namespace`, and `validator_primary_party`. Together the
  two commands let downstream scripts (e.g. a consumer's
  `allocate-parties-localnet.sh`) stop hardcoding realm names, client
  ids, client secrets, or the `<prefix>975` port scheme. Secrets are
  resolved from `compose/modules/keycloak/env/<slot>/on/oauth2.env` by
  default so rotations in this repo flow to consumers automatically;
  per-slot env-var overrides (ADR-0003 names) win when set.
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
- Hetzner Cloud LocalNet VM Terraform module (`terraform/hetzner/`)
  (#80): a part-time CCX33 server gated by `server_enabled`, brought up
  on weekday mornings and deleted each evening (delete-not-poweroff) by
  `.github/workflows/hetzner-localnet-schedule.yaml`. Ledger state
  persists on a retained ext4 volume (Docker `data-root` relocated onto
  it) behind a retained primary IP across the nightly recreate. A
  `LOCALNET_PAUSED` repo variable forces the VM down for holds.
- Keyless CI Terraform-state access via AWS OIDC
  (`terraform/github-oidc/`) (#80): an IAM role whose trust policy is
  scoped to this repo's `dev` branch and `localnet-infra` GitHub
  environment, replacing long-lived AWS keys in CI.

### Changed

- `EndpointDiscovery` now orders `LocalnetProfile` as sv, a, b, c, d
  across the enum and every `switch`, and the sv-validator-1 slot no
  longer ships fabricated OAuth2 defaults. Its default token URL and
  client id join its (already default-less) client secret as required
  env — no Keycloak realm is provisioned for the SV slot (the keycloak
  module ships realms for a/b/c/d only), so the previous `sv-validator`
  client id and `sv-validator-1` realm token URL pointed at endpoints
  that do not exist. `Resolve`/`ResolveForSlot` for the SV slot now
  throw a clear "set
  `CANTON_LOCALNET_SV_VALIDATOR_1_{TOKEN_URL,CLIENT_ID,CLIENT_SECRET}`"
  error instead of returning unusable defaults that 404 at token time.
  `IsSlotAvailable`/`IsLocalnetAvailable` now also require the SV token
  URL before reporting the SV slot available, keeping the availability
  gate in lock-step with what `Resolve` can resolve so a gated SV
  integration test skips cleanly instead of throwing. The a/b/c/d
  defaults are unchanged.
- **BREAKING (AWS Terraform root module):** the `terraform/` AWS stack
  is now self-contained — it clones `canton-localnet` and runs `make up`
  directly instead of invoking a consumer repo's `install.sh`/`deploy.sh`
  (#80). The `consumer_repo` / `github_token` / `localnet_branch`
  variables are replaced by `repo_url` / `repo_ref` / `repo_token`;
  update any `.tfvars` accordingly.

- DAR-upload integration tests run for the first time in CI (#60).
  Both Go and C# fixtures self-skip the DAR-upload portion on empty
  `CANTON_LOCALNET_TEST_DAR_PATH` for local-dev ergonomics, and the
  workflow previously tried to extract a DAR at CI time via
  `docker cp` from the splice-onboarding container — but that
  container has never carried DARs in this stack (built from
  `alpine + jwt-cli + jq + curl + bash`; see
  `compose/modules/splice-onboarding/docker/Dockerfile`), and neither
  does the `canton` container at our current `IMAGE_TAG=0.6.2`
  (`/canton/dars` does not exist on its filesystem; the bundled DARs
  appear to be embedded inside the Canton jar). The extract step
  therefore always hit a `::warning::no DAR files inside container …`
  branch and `exit 0`'d, keeping CI green on a permanent silent skip
  ever since #26 introduced the step. Replace the extraction with a
  CI-time `daml build` of a 5-line `Noop.daml` under `testdata/noop/`
  using the same Daml SDK (3.4.11) the rest of the Peaceful Studio
  stack uses, so the produced DAR is guaranteed to be within Canton
  0.6.2's accepted Daml-LF range (2.1..2.2). The SDK install is
  cached under `~/.daml` so subsequent runs incur only the build cost
  (~1s). The Go and C# integration steps now read the freshly built
  DAR via `${{ steps.build-dar.outputs.dar_path }}`, exercising
  `UploadDar` / `UploadDarAsync` end-to-end. No binary DAR is
  vendored in-tree.

- `Peaceful.Canton.Localnet.Testing` NuGet packaging now conforms with
  the `peacefulstudio/dotnet-extensions` baseline: the package embeds
  its README (`PackageReadmeFile`, fixing the missing-readme pack
  warning), ships a `.snupkg` symbol package with Source Link
  (`Microsoft.SourceLink.GitHub`, `EmbedUntrackedSources`,
  `PublishRepositoryUrl`), pins `AssemblyVersion`/`FileVersion`, and
  packs to `output/nuget`. Prepares the package for publication to
  nuget.org via the shared `csharp-publish-public.yaml` workflow.

### Fixed

- `canton-localnet up` no longer aborts with `dependency failed to
  start: container splice is unhealthy` when splice is merely slow to
  pass its `readyz` healthcheck on a loaded runner. Bumped the splice
  healthcheck `start_period` from `30s` to `600s` in
  `compose/modules/localnet/compose.yaml`. While a container's health
  status is `starting`, Compose's `up` dependency-waiter keeps waiting;
  once `start_period` elapses, the first failing probe flips the status
  to `unhealthy` and the waiter aborts the dependents (`nginx`,
  `splice-onboarding`), exiting 1 before the generous `wait-ready`
  step ever runs. The previous `30s` window was far shorter than
  splice's real readiness time (CI budgets 600s per validator), so the
  gate tripped intermittently. The high `retries: 1000` is unchanged;
  a genuine container crash still surfaces immediately because the
  waiter errors on an exited container. This removes the need for the
  consumer-side bring-up retry that papered over the race.
- Right-sized canton / splice JVM heaps and missing PQS caps for the
  full 5-validator topology (#72). The `canton` and `splice` JVMs each
  host every enabled slot's participant / validator app, so heap
  pressure scales linearly with slot count. Previous values (canton
  `-Xmx2560m` / `mem_limit 4g`, splice `-Xmx2560m` / `mem_limit 3g`)
  predated the c-/d-validator additions and OOM-killed the splice
  container on cold start (exit 137) and accumulated `RestartCount`
  on the canton container at idle on a 5-slot stack. New values
  (canton `-Xmx4g` / `mem_limit 5g`, splice `-Xmx3g` / `mem_limit 4g`)
  follow the rule of thumb "~+500 MB/validator for splice across the
  range and for canton up to 3 slots, ~+750 MB/validator for canton
  above 3 slots" now documented inline in
  `compose/modules/localnet/resource-constraints.yaml` and in the new
  "Resource sizing" section of `CONTEXT.md` (with a Docker Desktop
  allocation table per topology, flagged as estimates pending a
  live-bring-up validation). Also adds the missing `pqs-sv-validator-1`
  cap to `compose/modules/pqs/resource-constraints.yaml` (a/b/c had
  blocks, sv ran uncapped) — `pqs-d-validator-1` is intentionally
  skipped because no PQS service for slot `d` exists in
  `compose/modules/pqs/compose.yaml` today. Bumps `postgres-metrics`
  `mem_limit` from `32mb` to `96mb` in
  `compose/modules/observability/compose.yaml` (idle was ~65% of
  32 MiB; would OOM under load). The CLI stale-observability-container
  guard from the same investigation is deferred to a follow-up issue.
- Scribe (PQS) image pin bumped from `0.6.11` to `0.6.13` in
  `compose/modules/pqs/compose.env`. Scribe `0.6.11` crash-loops on
  any participant carrying a Daml LF 2.2 package with
  `java.util.NoSuchElementException: key not found: LanguageMinorVersion(2)`,
  which surfaced as `pqs-a-validator-1` failing to start once Splice's
  own onboarding wrote LF 2.2 events into the ledger stream. The
  override is still honoured (`SCRIBE_VERSION=… canton-localnet up`),
  so downstream consumers can pin back if needed.

### Security

- Scope the CI Terraform-state OIDC role to a single branch and
  environment (#80): the trust policy now requires the `aud`, `sub`
  (`…:environment:localnet-infra`), and `ref` (`refs/heads/dev`) claims
  together, replacing a `repo:…:*` wildcard, and the state-access policy
  is scoped to the Hetzner key prefix rather than the whole bucket.
- Pass the repo clone token via a per-invocation
  `git -c http.extraHeader=…` on both the AWS and Hetzner provisioners
  instead of embedding it in the clone URL (#80); the token is never
  written to `.git/config` or the boot log.

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
  glossary), architecture decision records, and
  `docs/public/MIGRATION.md` (terse before/after table for consumer
  migration).

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
  `docs/public/canton-localnet-yaml-schema.md`.

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
  5-slot topology as "next iteration", and `docs/public/MIGRATION.md`
  calls out that `c`/`d` are new slots without a "before" form. The
  README YAML example and the `docs/public/canton-localnet-yaml-schema.md`
  example
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
  mirror the legacy `tunnel.sh` scripts in downstream consumers and
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
  recorded in an architecture decision record;
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
  **preview / unstable** until compose codegen lands (per the
  recorded architecture decision). Breaking
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
  the package into a consumer test suite, which uses xunit 2.
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
  downstream consumers and `terraform-provider-canton` CI carry today.
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
  configuration migrated from an internal Peaceful Studio
  terraform module. Backend points at the shared CI state
  bucket at a new state key
  `canton-localnet/vm/terraform.tfstate`; the consumer state at
  the consumer's own state file is left intact. Resource names
  / SG name / key-pair name preserve the legacy resource prefix so
  `terraform import` produces a clean plan; the `Project` tag is the
  only intentional value change (legacy prefix → `canton-localnet`), used
  to scope the CI IAM policy. Variable names and output names
  (`instance_id`, `elastic_ip`, `ssh_command`, `ssh_key_path`,
  `region`, `ami_id`) match the consumer so its tunnel scripts work
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
