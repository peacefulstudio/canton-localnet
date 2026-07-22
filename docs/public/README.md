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
| CLI lifecycle | `up` / `down` / `wait-ready` / `auth token` / `info` / `vm` |
| Flexible topology | 1 SV + N validators across 5 named slots; SV-only → all-five |
| Programmatic fixtures | C# `LocalnetFixture` + Go `fixture`: query + mutate a live ledger |
| Observability & PQS | Grafana/Prometheus/Loki/Tempo + per-slot Participant Query Store |
| Multi-synchronizer | Second synchronizer profile for cross-domain scenarios |
| Infra | AWS spot + part-time Hetzner Terraform, scheduled up/down |
| Release artifacts | NuGet + Go module + CLI binaries + OCI compose artifact, in lockstep |

## Guides

- [Getting started](../../README.md) — quickstart, ports, CLI, and configuration.
- [Topologies](topologies.md) — the role model, the five named slots, optional layers, and the scenario/config matrix.
- [Integration testing](integration-testing.md) — the attach model, the C# and Go fixtures, the env-var contract, and CI wiring.
- [Config schema](canton-localnet-yaml-schema.md) — the full `canton-localnet.yaml` reference.
- [Migration](MIGRATION.md) — moving an existing setup onto the current config surface.
