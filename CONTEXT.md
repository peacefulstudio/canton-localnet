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
