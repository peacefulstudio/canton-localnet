# canton-localnet — Domain Glossary

## Topology roles

The LocalNet topology is composed of **roles** — there are exactly two:

- **SV** (Super Validator) — runs the synchronizer infrastructure (sequencer + mediator + scan) alongside the SV splice app. There is exactly one SV per synchronizer. Today's stack has one SV. A future cross-synchronizer topology will introduce a second SV (deferred, out of current scope).
- **Validator** — a participant + Splice ValidatorApp + wallet/ans web UIs, hosting one or more parties. A **Witness** is a Validator whose hosted parties exist specifically to assert privacy boundaries in tests ("party W on the witness validator should NOT observe transactions between parties on alice/bob validators"). It is not a separate role and not parameterised differently from other validators — its witness-ness is a usage pattern, not a configuration shape.

A topology is one SV plus N Validators. The current stack ships **5 slots** — `sv-validator-1` plus four general-purpose validator slots (`a-validator-1`, `b-validator-1`, `c-validator-1`, `d-validator-1`) — with a per-slot **enable flag** so consumers can boot only the validators they need. Slot `d` is reserved for the witness use case.

**SV is non-toggleable** — every topology has exactly one SV. The minimum bootable topology is `sv-validator-1` alone (a=b=c=d disabled, N=0). The default is everything on (N=4, full 5-slot topology). Topologies size between these two extremes by toggling individual validator slots.

## Naming format

Every validator (SV included) follows the format `<name>-<function>-<index>`:

- `<function>` is `validator` (canton-localnet treats SV as a specialised validator at the naming level).
- `<index>` is a 1-based integer that disambiguates multiple validators sharing the same `<name>`.
- `<name>` carries semantic meaning. There are two scopes for `<name>`:
  - **Slot name** — baked into the compose graph and the filesystem. Letter-coded to stay domain-neutral across consumers: `sv`, `a`, `b`, `c`, `d`. So the canton-localnet repo ships `sv-validator-1`, `a-validator-1`, `b-validator-1`, `c-validator-1`, `d-validator-1` as slot identifiers in `compose/modules/localnet/conf/{canton,splice,console}/<slot>/`.
  - **Party hint** — supplied per-consumer via `canton-localnet.yaml`. Maps each slot to a consumer-meaningful name, e.g. for murmures: `a-validator-1` → party hint `featuredapp-validator-1`, `b-validator-1` → `alice-validator-1`, `c-validator-1` → `bob-validator-1`. Other consumers map differently (terraform-tests: `a-validator-1` → `tf-validator-1`).

The split keeps the shared artifact domain-neutral while letting consumers express their own domain.

## Configuration discovery

`canton-localnet.yaml` is the per-consumer config file. The CLI discovers it by walking up from the current working directory until it finds a `canton-localnet.yaml` or hits the filesystem root (same pattern as `git`, `docker compose`, `kubectl`). A `--config <path>` flag overrides the walk-up. With no config found anywhere, the CLI uses built-in defaults (all 5 slots on, obs + pqs on, default slot party hints). Partial configs merge over defaults; references to unknown slots (`e-validator-1` etc.) are hard errors.

## Resource sizing

The `canton` and `splice` services each run a single JVM hosting **all** enabled validator slots' participants / validator apps. Heap demand therefore scales linearly with the number of enabled slots — splice grows ~500 MB per added slot across the full range, canton grows ~500 MB per slot up to 3 slots and ~750 MB per slot above that. The shipped caps in `compose/modules/localnet/resource-constraints.yaml` are sized for the default 5-slot topology (sv + a + b + c + d):

| Topology              | canton `-Xmx` / `mem_limit` | splice `-Xmx` / `mem_limit` |
|-----------------------|-----------------------------|-----------------------------|
| 1 validator (sv only) | 1.5g / 2.5g                 | 1g / 1.5g                   |
| 3 validators (sv+a+b) | 2.5g / 3.5g                 | 2g / 2.5g                   |
| 5 validators (default)| 4g / 5g                     | 3g / 4g                     |

### Docker Desktop allocation

The figures below are **estimates** — they have not been empirically validated post-resize. The 5-validator caps were verified end-to-end at the rendered-config level (`make config` parses cleanly with the new values wired through), but the bring-up acceptance criteria (`make up` succeeds, idle `RestartCount == 0`, mem% < 60) still need a live run on a 16 GiB allocation to confirm.

| Topology                            | Recommended | Headroom-for-dev |
|-------------------------------------|-------------|------------------|
| 3-validator                         | 8 GiB       | 12 GiB (room for builds)              |
| 5-validator + observability         | 16 GiB      | 24 GiB (room for `make build` + tests) |

Idle steady-state working set for the full 5-validator stack with PQS + observability sits around 6.5 GiB; anything tighter than 16 GiB leaves zero headroom for Daml compilation (~3–4 GB) or test runs on top.
