# canton-localnet

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A shared, reusable, flexibly-topological Canton LocalNet that Peaceful Studio's
repositories build their integration tests on — compose + xUnit/Go fixtures +
CLI + AWS terraform.

## Capabilities

| Capability | What ships today |
|---|---|
| Declarative config | Single-file `canton-localnet.yaml` (topology, slots, parties, auth) |
| CLI lifecycle | `up` / `down` / `wait-ready` / `auth token` / `info` / `vm` |
| Flexible topology | 1 SV + N validators across 5 named slots; SV-only → all-five |
| Programmatic fixtures | C# `LocalnetFixture` + Go `fixture`: query + mutate a live ledger |
| Observability & PQS | Grafana/Prometheus/Loki/Tempo + per-slot Participant Query Store |
| Multi-synchronizer | Second synchronizer profile for cross-domain scenarios |
| Infra | AWS spot + part-time Hetzner Terraform, scheduled up/down |
| Release artifacts | NuGet + Go module + CLI binaries + OCI compose artifact, in lockstep |

## Use cases

- Local multi-participant app development against a real Canton ledger.
- Integration and end-to-end testing.
- CI/CD pipelines.
- Privacy-boundary / witness testing (the reserved `d-validator-1` slot).
- Multi-synchronizer / cross-domain scenarios.
- Demos and runtime samples.

It underpins our own repositories' integration suites — see the
[integration-testing guide](docs/public/integration-testing.md) — and the
[topologies guide](docs/public/topologies.md) shows how far you can push the
topology.

## Quickstart

Boot a complete Canton LocalNet (Splice 0.7.5) on your machine:

```bash
make up           # docker compose up -d, OAuth2 mode by default
make wait-ready   # poll the JSON Ledger API until participant accepts requests
make down         # stop the stack and remove containers
```

JSON Ledger API endpoints once ready (see `compose/modules/localnet/env/common.env`):

| Slot             | JSON Ledger API |
|------------------|-----------------|
| `sv-validator-1` | 10975           |
| `a-validator-1`  | 11975           |
| `b-validator-1`  | 12975           |
| `c-validator-1`  | 13975           |
| `d-validator-1`  | 14975           |

Slot ports follow the 5-digit two-digit-prefix scheme (`<prefix><suffix>`):
each slot's participant ledger / admin / JSON / Splice validator admin ports
share the same two-digit prefix as the JSON Ledger API port shown above.

Optional layers via Make flags (shortcuts — the canonical config layer is
`canton-localnet.yaml`, see **Configuration** below):

```bash
make up PQS=true            # opt in to PQS a-validator-1 profile
make up OBS=true            # add Grafana (http://localhost:3030) + Prometheus / Loki / Tempo / cAdvisor
make up RES=false           # remove the default mem_limit / JVM heap caps
make up AUTH_MODE=secret    # shared-secret JWT (escape hatch; not CI-tested)
```

When iterating with observability on, `make stop-app` (and `make clean-app`)
restart the application stack while leaving Grafana / Prometheus / Loki / Tempo
running so dashboards stay populated.

### Remote VM (AWS)

For consumers that prefer running LocalNet on a shared EC2 instance, the
`vm` subcommand wraps the `terraform/` stack and an ssh tunnel:

```bash
canton-localnet vm provision        # terraform apply, prints public IP + ssh command
canton-localnet vm tunnel           # ssh -L 11901, 7575, 8082 to the VM (Ctrl-C to close)
canton-localnet vm destroy --yes    # terraform destroy (interactive prompt without --yes)
```

The tunnel forwards the same port set the legacy `tunnel.sh` scripts in
downstream consumers and `terraform-provider-canton` open: `11901` (a-validator-1
participant gRPC ledger API), `7575` (legacy in-container Splice JSON
Ledger API), and `8082` (Keycloak). `vm provision` is idempotent —
re-running on an already-applied state is a no-op refresh.

> The 5-digit port renumbering applies to the host-exposed
> port set on the local stack (`10975`/`11975`/… for JSON Ledger API).
> The `vm tunnel` default port set is unchanged for backwards
> compatibility with the legacy tunnel scripts and is documented as
> such in [`cli/internal/tunnel`](cli/internal/tunnel/tunnel.go). If
> you connect to JSON Ledger API on the VM, forward the corresponding
> host port (`11975` for `a-validator-1`, etc.) explicitly.

Prerequisites: Docker ≥ 27, Docker Compose ≥ 2.27. The compose stack is
vendored into `compose/modules/` from `hyperledger-labs/splice` at the SHA
pinned in `compose/splice.sha`. Re-vendor with `make vendor`.

### Multi-synchronizer profile

Bring up a second synchronizer (`app-synchronizer`) that the `a`/`b`/`d`
validators connect to, for cross-synchronizer tests:

```bash
canton-localnet up --multi-sync
canton-localnet wait-ready --synchronizers 2   # blocks until a-validator-1 sees both
# or, via make:
MULTI_SYNC=true make up
```

Discover the second synchronizer's id from a test (C#):

```csharp
var party = await fixture.AllocatePartyAsync("lapi");
var appSyncId = await fixture.GetAppSynchronizerIdAsync(party.PartyId);
// pin a submission: WithSynchronizerId(new SynchronizerId(appSyncId))
```

`multi-sync` is local/CI only — it is not enabled on the shared VM.

## Driving a slot from a script

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
  "participant_id": "a-validator-1::1220...",
  "participant_namespace": "1220...",
  "validator_primary_party": "a-validator-1::1220..."
}
```

Pass `--offline` to skip the live participant-id lookup when you only
need the static endpoint mapping (e.g. before the stack is up). Short
slot names (`a`, `b`, `c`, `d`, `sv`) and canonical names
(`a-validator-1`, …) are both accepted.

## Pruning leaked ledger rights

A Canton participant caps a user at 1000 rights and never deletes a
party, so every `CanActAs` grant an integration suite makes on an
ephemeral party is permanent. Suites that revoke on teardown leave
nothing behind; a run killed before teardown does, and a long-lived
shared LocalNet silts up until further grants fail with
`TOO_MANY_USER_RIGHTS`.

```bash
# Audit: what does the slot's validator user hold?
canton-localnet rights list --slot a
canton-localnet rights list --slot a --full   # name every party, don't group

# Sweep: print the plan, then revoke after confirmation.
canton-localnet rights prune --slot a
canton-localnet rights prune --slot a --dry-run --full   # plan it, name every party
```

or `make list-rights SLOT=a` / `make prune-rights SLOT=a` (add `YES=1`
to skip the prompt; `YES=0` and an unset `YES` both keep it). `prune`
writes nothing unless it is given `--yes` or answered at an interactive
prompt.

`rights list` groups the ephemeral parties by the hint their allocator
used, so a user carrying hundreds of leaked grants reads as a handful of
counted lines. Pass `--full` to name every party instead — that is how
the source of a leak gets identified.

```
slot             a-validator-1
json_api         http://localhost:11975
user             c87743ab-80e0-4b83-935a-4c0582226691
preserving       ParticipantAdmin, and CanActAs on a-validator-1::1220ab…*
preserve source  the user's primary party, read from the participant
total rights     53
preserved        2
    keep    ParticipantAdmin
    keep    CanActAs                 a-validator-1::1220ab…
to revoke        51
    revoke    46  CanActAs                 grpc-writer-parity-*
    revoke     5  CanActAs                 rest-writer-parity-*
```

`rights prune` is destructive. Its full contract is in `canton-localnet
rights prune --help`, and the plan it prints on every run restates the
part that matters — what is preserved, and where that came from. In
short: it keeps the `ParticipantAdmin` right and every `CanActAs` right
on the validator's own party, and revokes everything else, other right
kinds on the validator's own party included.

The preserved party is read from the participant, as the target user's
`primaryParty`. That matters because it is not guessable from the slot
name: `sv-validator-1` operates as the founded party `sv::<ns>`, not
`sv-validator-1::<ns>`. Two refusals stand behind it, and neither can be
waived:

- **No `ParticipantAdmin` right** — the command is aimed at a user whose
  rights are not disposable.
- **No `CanActAs` right on the preserved party** — every validator user
  holds one, so its absence means the preserved party is wrong for this
  slot and the sweep would take the user's own act-as rights with it.

Both refusals gate anything destructive, so neither can be waived when
there is something to revoke. A user with an empty revoke list never
reaches them; that path reports the user as clean and warns when the
preserved party looks wrong for the slot.

Use `--preserve-party <party>` when the participant reports no primary
party. The value must be a full party id containing `::`, because a bare
hint such as `a-validator-1` also prefix-matches `a-validator-1-foo::…`,
and the user must still hold an act-as right on it — a typo refuses
exactly as a wrong slot does. It follows that a user this tool has
already stripped of its own act-as right cannot be pruned again; the
repair for that is to re-grant the right.

The rest of the rails:

- `--max-revoke` (default 1000) aborts a runaway list. It sits at the
  participant's own per-user cap, so on a stock participant it does not
  fire; it is a sanity bound, not a protection, and it is deliberately
  not lower, because recovering a participant saturated at 999 rights is
  the case this exists for.
- The rights are re-read live immediately before revoking, and the
  revoke proceeds only if that live set is contained in the plan that
  was consented to. If new rights appeared in between — likely on a busy
  shared box — it aborts and asks you to re-run rather than revoke more
  than was agreed.
- It fails loudly when the participant reports revoking fewer rights
  than were asked for.
- It is safe to re-run: with nothing ephemeral left it exits 0.
- `--dry-run` plans, runs every check, and exits 0 without writing, so
  its `0` asserts the plan is executable rather than merely printed. It
  is mutually exclusive with `--yes`, as a hard error rather than a
  precedence rule.
- `--full` works on `prune` too, so the plan you authorise can name
  every party rather than summarise them.

### Exit status (`prune`)

| code | meaning |
| --- | --- |
| `0` | nothing to revoke, a successful revoke, or a `--dry-run` whose plan passed every check |
| `2` | the sweep was not authorised: the prompt was declined, or `--yes` was withheld from a non-interactive run |
| `1` | everything else — a rail refused, the live set drifted past what was consented to, a call failed, or the invocation itself was rejected before any ledger call (a missing or unparseable `--slot`, `--yes` together with `--dry-run`, a `--preserve-party` without `::`, an unresolved validator user id, a `--repo-root` with no compose tree) |

**`2` always means nothing was changed. `1` does not.** A partial revoke
and a failed read-back after a successful revoke both exit `1` with
rights already gone. On `1`, re-run `rights list` before assuming the
state.

`2` is distinct on purpose, so a `prune || alert` wrapper does not page
when somebody deliberately answers no, and a CI job that forgets `--yes`
cannot report a green sweep while the box stays saturated.

The user id comes from `AUTH_<SLOT>_VALIDATOR_USER_ID` in
`compose/modules/keycloak/env/<slot>/on/oauth2.env`, overridable with
`CANTON_LOCALNET_<SLOT>_USER_ID`.

> **Party hints.** The CLI reads `CANTON_LOCALNET_<SLOT>_PARTY_HINT`,
> while compose reads the unprefixed `<SLOT>_PARTY_HINT`
> (`compose/modules/localnet/env/<slot>-auth-on.env`). A stack booted
> with a custom hint set only in the compose form leaves the CLI on the
> default. That only reaches `prune` as the fallback when the
> participant reports no primary party, and the no-own-party refusal
> covers it — but set both forms if you customise a hint.

## Configuration

The CLI reads an optional `canton-localnet.yaml` from the working
directory or any ancestor (walk-up discovery, matching `git` / `docker
compose` / `kubectl`), overridable with `--config <path>`. With no
file found anywhere, the CLI uses built-in defaults (all five
validator slots enabled, observability + PQS on, party hints equal to
slot names). Partial configs merge over the defaults.

```yaml
schemaVersion: preview-1
modules:
  obs: true
  pqs: true
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
    auth: { clientId: a-validator-1-validator, clientSecret: ${FEATUREDAPP_VALIDATOR_SECRET} }
  c-validator-1: { enabled: false }
  d-validator-1: { enabled: false }
```

The schema is **preview / unstable** until compose codegen lands —
see [`docs/public/canton-localnet-yaml-schema.md`](docs/public/canton-localnet-yaml-schema.md)
for the full reference.

## Onboarding clients

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

## Project stewardship

`canton-localnet` is currently developed and maintained by **Peaceful Studio
OÜ** (Estonia, VAT EE102232996). The project is licensed under Apache-2.0
with the explicit intent of community ownership: if and when adoption
warrants neutral governance, Peaceful Studio commits to transferring this
repository to a community-led organisation under the same license terms.
Contributions welcome from anywhere in the Canton, Daml, and Splice
ecosystem; no CLA required.

## Contributing

Contributions are welcome from anyone in the Canton, Daml, and Splice
community. See [CONTRIBUTING.md](CONTRIBUTING.md) for the dev setup,
the red-green TDD requirement, and the branch model. The per-PR
checklist itself lives in the PR template and is filled in when you
open a PR. By participating you agree to abide by the
[Code of Conduct](CODE_OF_CONDUCT.md).

For security-sensitive bugs, please follow [SECURITY.md](SECURITY.md) instead
of opening a public issue.

## License

Apache-2.0. © 2026 Peaceful Studio OÜ. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
