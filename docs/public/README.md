<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# canton-localnet documentation

A shared, reusable, flexibly-topological Canton LocalNet that Peaceful Studio's
repositories build their integration tests on. Start with the map below, then
follow the guide that matches your task.

## Capabilities

| Capability | What ships today |
|---|---|
| Declarative config | Single-file `canton-localnet.yaml` (topology, slots, parties, auth) |
| CLI lifecycle | `up` / `down` / `wait-ready` / `auth token` / `tap` / `info` / `tunnel` |
| Flexible topology | 1 SV + N validators across 5 named slots; SV-only → all-five |
| Programmatic fixtures | C# `LocalnetFixture` + Go `fixture`: query + mutate a live ledger |
| Observability & PQS | Grafana/Prometheus/Loki/Tempo + per-slot Participant Query Store |
| Multi-synchronizer | Second synchronizer profile for cross-domain scenarios |
| Infra | Part-time Hetzner Terraform, scheduled up/down |
| Release artifacts | NuGet + Go module + CLI binaries + OCI compose artifact, in lockstep |

## Guides

- [Local quickstart](quickstart.md) - boot a LocalNet on your machine: ports, optional layers, the tunnel to a remote LocalNet and the multi-synchronizer profile.
- [GitHub Action](github-action.md) - run a LocalNet in your own CI and read the exported endpoints and credentials.
- [Integration testing](integration-testing.md) - the attach model, the C# and Go fixtures, the env-var contract, and CI wiring.
- [Topologies](topologies.md) - the role model, the five named slots, optional layers, and the scenario/config matrix.
- [Config schema](canton-localnet-yaml-schema.md) - the full `canton-localnet.yaml` reference.
- [Scripting](scripting.md) - mint tokens and read endpoints from a script with `auth token` and `info`.
- [Funding a validator party with Amulet](funding-amulet.md) - `tap`, including `--at-least`.
- [Pruning leaked ledger rights](pruning-rights.md) - `rights list` and `rights prune`, with their exit status.
- [Onboarding clients](onboarding-clients.md) - the per-realm OAuth client that manages users.
- [Migration](MIGRATION.md) - moving an existing setup onto the current config surface.

The [repository README](../../README.md) is the landing page: a short quick start and a table that points here.
