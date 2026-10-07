<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# The canton-localnet GitHub Action

Two composite actions boot and tear down the LocalNet compose stack in a
GitHub Actions job, without a checkout of this repository and without any
App token:

- `peacefulstudio/canton-localnet@<tag>` — boot: pre-boot cleanup, `up`
  with one retry, `wait-ready`, then a single
  `canton-localnet env --format github` call that exports every enabled
  slot's endpoints and credential contract into `$GITHUB_ENV`, masked.
- `peacefulstudio/canton-localnet/teardown@<tag>` — teardown: `down --volumes`.
  Call it with `if: always()` right after your test steps. On failure it
  uploads bounded diagnostics and **exits non-zero** — unlike the
  `down || echo ::error::` pattern some lanes used before this action,
  which always read green even when teardown actually failed.

Pin the action to a release tag or its commit SHA (never a moving ref);
see [`RELEASE.md`](../../RELEASE.md) for this repository's version scheme.
Note the one exception: `cli: release` resolves the binary from
`github.action_ref`, which is only ever a literal tag when the action's
own `uses:` is pinned to that tag — a SHA-pinned `uses:` resolves
`action_ref` to the SHA, not the tag it points at, so `cli: release`
fails closed on a SHA pin. If you SHA-pin (the safer default), use
`cli: source` (the default); reserve `cli: release` for a tag-pinned
`uses:`.

Run one LocalNet per Docker daemon. The stack uses fixed container names,
a fixed `localnet` network, and fixed host ports, so two concurrent
LocalNets on the same daemon collide. The action's own pre-boot cleanup
only removes the `localnet` compose project on the daemon it runs
against — it does not, and cannot, guard against a second, independent
job racing it on the same self-hosted runner.

## Minimal consumer example

```yaml
jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6.0.3

      - name: Boot LocalNet
        id: localnet
        uses: peacefulstudio/canton-localnet@<sha> # v<ver>
        with:
          validators: a

      - name: Run your tests against the exported endpoint/credentials
        run: your-test-command
        # CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL, _TOKEN_URL, _CLIENT_ID
        # and _CLIENT_SECRET (masked) are already in the job environment —
        # read them from there, don't `echo` or otherwise print them.

      - name: Tear down
        if: always()
        uses: peacefulstudio/canton-localnet/teardown@<sha> # v<ver>
```

**Never `echo`, `printenv`, or otherwise print an exported `_CLIENT_SECRET`,
`_JWT` or `_PQS_CONNECTION_STRING` value in your own step.**
GitHub Actions masks the literal value in the raw log once it has seen it
via `::add-mask::`, but that masking does not follow the value into
anything your own step captures and writes somewhere else — a file it
uploads as an artifact, a step summary it composes by hand, or an
in-process log line. Read the variable straight from the environment at
the point you need it, and let your test tooling do the same.

This action boots its own Keycloak with the fixed LocalNet demo client ids
and secrets (`compose/modules/keycloak/env/<slot>/on/oauth2.env`) and does
not reconfigure it to match a `CANTON_LOCALNET_<SLOT>_CLIENT_ID` or
`CANTON_LOCALNET_<SLOT>_CLIENT_SECRET` override — those overrides are
`canton-localnet env`'s own per-slot credential escape hatch for running it
by hand against an externally managed LocalNet stack. If either is set in
the job environment to a non-blank value that differs from the fixed demo
value when this action runs, it fails closed before booting rather than
exporting a client id or secret Keycloak would reject. A blank override, or
one already equal to the fixed demo value — for example because a prior
`uses:` of this action in the same job already exported it — is not an
error.

## Inputs

| Input | Default | Meaning |
|---|---|---|
| `validators` | `a` | Space- or comma-separated slots to bring up, from `a`, `b`, `c`, `d`. `sv-validator-1` is always on and cannot be listed. |
| `pqs` | `false` | Enable the Participant Query Store module; adds each enabled slot's PQS connection details to the export. Only `a` and `c` actually get a running PQS pipeline (`cli/internal/compose/compose.go`) — a `b` connection string points at a database no pipeline writes to, and the action warns if you combine `pqs: true` with a `b` slot. Slot `d` is rejected outright by the CLI. |
| `observability` | `false` | Enable the observability stack (Grafana/otel). |
| `multi-sync` | `false` | Enable the multi-synchronizer profile and wait for the app-synchronizer to connect. Requires `a`, `b` and `d` all present in `validators` — the app-synchronizer console script waits on all three — and the action fails closed if any are missing. A `config` file whose top-level `multiSync` is truthy also enables this wait, even when this input is left at its default. |
| `dialect` | `native` | Exported variable dialect. Only `native` (`CANTON_LOCALNET_*`) ships today; `devkit` is not yet supported and the action fails the step rather than exporting a wrong or partial contract. |
| `roles` | *(empty)* | Devkit-dialect role mapping. Only meaningful with `dialect: devkit`, which is not yet supported — leave unset. |
| `jwt` | `false` | Mint a bearer token per enabled slot and export it plus the live participant id. Off by default: the token is not exported, and unless `party` is `true` no token is minted at all. The token URL, client id and client secret are exported either way. |
| `party` | `false` | Export each enabled slot's validator primary party as `CANTON_LOCALNET_<SLOT>_PARTY` (for `a`: `a-validator-1::1220…`), read from the validator user (`GET /v2/users/{id}` → `primaryParty`) after a token mint. The validator user holds `CanActAs` on that party. Off by default. The token is minted for the lookup only; it is exported only with `jwt: true`. |
| `timeout` | `15m` | One total deadline shared by pre-boot cleanup, `up` (with its one retry) and `wait-ready` together — not a per-slot timeout. |
| `up-timeout` | empty | Longest one `up` attempt may take (e.g. `15m`) before it is killed and the one retry starts. Empty or zero lets an attempt use whatever is left of `timeout`; set it below `timeout` to keep room for the retry, because LocalNet's own health check retries for hours and a stalled boot otherwise spends the whole budget. |
| `cli` | `source` | `source` builds the CLI with `go build` from the action's own checkout (~9s measured with a warm Go module cache). `release` downloads the release asset matching the action's pinned ref and verifies it against that release's `checksums.txt`; valid only when the action's own `uses:` is pinned to a literal release tag (not its commit SHA — `github.action_ref` resolves to whichever form the pin used), and it fails closed — never falls back silently — on a non-tag ref, a missing asset, a download error, or a checksum mismatch. |
| `config` | *(empty)* | Path (relative to your workspace) to a `canton-localnet.yaml` to use verbatim, instead of the one generated from `validators`/`pqs`/`observability`/`multi-sync`. When set, also set `validators` (and `pqs`) to match what that file actually enables — `wait-ready` and the env export still key off `validators`, not off parsing your file. |

## Outputs

| Output | Meaning |
|---|---|
| `cli-path` | Absolute path to the resolved `canton-localnet` binary. |
| `profile` | Canonical name of the primary exported slot (the first entry of `validators`; also `CANTON_LOCALNET_PROFILE` in `$GITHUB_ENV`). |

Everything else — endpoints, ports, the credential contract, and (with
`pqs: true`) PQS connection details — is exported only as `$GITHUB_ENV`
variables, never as action outputs, because action outputs are visible in
the job's API-readable metadata and several of these values are secrets.
See [`integration-testing.md`](integration-testing.md) for the full
per-slot variable contract `canton-localnet env` renders.

## Job summary

Every run leaves a compact report in the job summary:

- a header with the action ref, the CLI version and the Splice version;
- a **Validators** table with one row per booted slot: JSON API, ledger gRPC,
  admin gRPC and validator API URLs, whether PQS is enabled, and the primary
  party when `party: true`;
- the **Shared endpoints**: the Scan URL, Keycloak and the OAuth token URL;
- **Timings** for the CLI resolve, pre-boot reconciliation, `up` (image pull
  and compose up, one phase), wait-ready, the PQS watermark wait (with
  `pqs: true`) and env export, plus the total;
- a **Teardown** section, appended by `.../teardown`: whether `down --volumes`
  succeeded, and any container that was unhealthy or had exited non-zero when
  teardown began. Their logs are uploaded in the
  `canton-localnet-teardown-diagnostics-<job>` artifact.

The tables are rendered by `canton-localnet env --format github
--summary-file <path>` from the same values it writes to `$GITHUB_ENV`, so the
report cannot disagree with the exports. It lists only endpoints: a client
secret, JWT, PQS password or PQS connection string never appears in it.

## Boot-failure diagnostics

When the boot fails, the action uploads the
`canton-localnet-diagnostics-<job>` artifact, kept for 7 days: container
status, recent compose logs, and the Docker networks and volumes. Its `host/`
folder holds `free -m`, `nproc`, the kernel out-of-memory lines from `dmesg`
and the peak memory used during the run and, for this LocalNet's compose
project only, `docker stats`, per-container state and health, and the tail of
the `splice` and `canton` logs. When the first `up` attempt also failed, a
`host-attempt1/` folder holds the same capture taken before the retry. Read
it to tell a runner that ran out of memory or CPU from a LocalNet fault.
Containers of other workloads on a shared Docker daemon are never listed or
logged.

## Admin gRPC and the Scan registry

Every exported slot carries `CANTON_LOCALNET_<SLOT>_ADMIN_GRPC_URL`, the
participant admin gRPC endpoint (`http://localhost:11902` for `a`), derived
from the same slot port prefix as `_GRPC_URL`.

The action also exports one global, `CANTON_LOCALNET_SCAN_URL`
(`http://scan.localhost:10000`), the token-standard registry
(`/registry/...`) and Scan API (`/api/scan/...`). nginx serves them only under
the `scan.localhost` virtual host of the SV web UI port, so the hostname must
resolve to loopback. `*.localhost` names resolve to `127.0.0.1`/`::1` through
`getaddrinfo` and `curl` on a developer machine, and systemd-resolved does the
same on `ubuntu-latest`, where the action's own self-test fetches
`$CANTON_LOCALNET_SCAN_URL/registry/metadata/v1/info` and expects a 200.
Where it does not resolve, add `127.0.0.1 scan.localhost` to `/etc/hosts`.

## Credentials and their lifetime

The action exports the OAuth2 credential contract (token URL, client id,
client secret) for every enabled OAuth2 slot (`a`, `b`, `c`, `d`) — always,
with no network call. `sv-validator-1` is booted and waited on like any
other slot, but `canton-localnet env` is OAuth2-only in v1 and SV has no
OAuth2 client, so the action exports no credential contract for it; mint
an SV token directly with `canton-localnet auth token --slot sv` if your
test needs one. `jwt: true` additionally mints a bearer token per enabled
OAuth2 slot: it lives 300 seconds (Keycloak's configured
`accessTokenLifespan`). A test run that outlives it should refresh via the
credentials (a `client_credentials` grant against `_TOKEN_URL`) rather than
assume a single minted token survives the whole run — every fixture and
live test suite in this repository's own consumers already does this.

## Funding a party with Amulet after boot

The action puts `canton-localnet` on `PATH` for later steps (its path is also
the `cli-path` output). A suite that needs an `Amulet` contract in PQS, such as
the Rust SDK's PQS live suite, funds the validator party with one step:

```yaml
- name: Fund slot a with Amulet
  run: canton-localnet tap --slot a --amount 10
```

With `pqs: true` the boot step does not return until Scribe has set its
watermark for each of `a` and `c`, so an `Amulet` tapped right after boot is
streamed into PQS with its effective time; no PQS polling step is needed in
your workflow.

`tap` needs only the booted stack: it uses LocalNet's demo wallet login and
prints the new contract id. It is safe to call immediately after boot: the
validator app can answer 429 or refuse connections for a few seconds while it
settles, and `tap` retries those with backoff for up to 60 seconds before
failing with the last status. The Keycloak login also retries 502, 503 and
504; the mint itself does not, since a mint may have committed behind them.
See the `tap` section of the repository README for the login, the amount
conversion and the refusals.

To leave a slot with a known balance regardless of earlier runs, use
`--at-least` in place of `--amount`; it taps only the gap and prints the
resulting balance, so a rerun is a no-op:

```yaml
- name: Fund slot a to 1000 Amulet
  run: canton-localnet tap --slot a --at-least 1000
```

## What the action does not do

- It does not check out this repository — the runner already has the
  pinned action's tree at `github.action_path`.
- It never mints or exports a devkit-dialect variable.
- It never writes a secret to `$GITHUB_OUTPUT`, an on-disk dotenv file, or
  a diagnostics artifact. Diagnostics collection never reads back the
  rendered env, `canton-localnet.yaml`, or a compose `oauth2.env` file.
