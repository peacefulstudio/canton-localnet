# canton-localnet

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A shared, reusable, flexibly-topological Canton LocalNet that Peaceful Studio's
repositories build their integration tests on: compose + xUnit/Go fixtures +
CLI + Hetzner terraform. Boot a real multi-participant Canton network on your
machine or in your CI with one command, and drive it from a config file, a CLI
or a test fixture.

| Capability | What ships today |
|---|---|
| Declarative config | Single-file `canton-localnet.yaml` (topology, slots, parties, auth) |
| CLI lifecycle | `up` / `down` / `wait-ready` / `auth token` / `tap` / `info` / `tunnel` |
| Flexible topology | 1 SV + N validators across 5 named slots; SV-only to all-five |
| Programmatic fixtures | C# `LocalnetFixture` + Go `fixture`: query + mutate a live ledger |
| Observability and PQS | Grafana/Prometheus/Loki/Tempo + per-slot Participant Query Store |
| Multi-synchronizer | Second synchronizer profile for cross-domain scenarios |
| CI | A composite GitHub Action that boots a LocalNet in your own workflow |
| Infra | Part-time Hetzner Terraform, scheduled up/down |
| Release artifacts | NuGet + Go module + CLI binaries + OCI compose artifact, in lockstep |

Typical uses: local multi-participant app development, integration and
end-to-end testing, CI/CD pipelines, privacy-boundary and witness testing,
multi-synchronizer scenarios, and demos. Running in CI? Jump to the
[GitHub Actions quickstart](docs/public/github-action.md#quickstart).

## Quick Start

Boot a complete Canton LocalNet (Splice 0.10.1) on your machine. You need
Docker >= 27 and Docker Compose >= 2.27:

```bash
git clone https://github.com/peacefulstudio/canton-localnet.git
cd canton-localnet
make up           # docker compose up -d, OAuth2 mode by default
make wait-ready   # poll the JSON Ledger API of every enabled slot until each accepts requests
make down         # stop the stack and remove containers
```

Once ready, the JSON Ledger API of each slot listens on `127.0.0.1`:
`sv-validator-1` on `10975`, `a-validator-1` on `11975`, `b-validator-1` on
`12975`, `c-validator-1` on `13975` and `d-validator-1` on `14975`.

The CLI offers the same lifecycle (`canton-localnet up`, `wait-ready`, `down`)
and more, as a binary on each
[release](https://github.com/peacefulstudio/canton-localnet/releases).

The stack ships fixed, public demo credentials, so run it on a loopback-only or
firewalled host and never against real assets. The full quickstart covers every
port, the optional layers (PQS, observability), the tunnel to a remote LocalNet
and the multi-synchronizer profile:
[docs/public/quickstart.md](docs/public/quickstart.md).

## Use it in GitHub Actions

The composite action `peacefulstudio/canton-localnet` boots a real Canton
LocalNet inside your own workflow, so your integration and CI/CD tests run
against an actual ledger instead of a mock. It needs no checkout of this
repository, exports each slot's endpoints and credentials to the job
environment (secrets masked), and a paired teardown action cleans up and
uploads diagnostics when something fails. It is language-neutral: your tests
only read environment variables.

```yaml
jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - name: Boot LocalNet
        uses: peacefulstudio/canton-localnet@v0.10.1-1
        with:
          validators: a
      - name: Run your tests
        run: your-test-command # reads CANTON_LOCALNET_A_VALIDATOR_1_* from the environment
      - name: Tear down
        if: always()
        uses: peacefulstudio/canton-localnet/teardown@v0.10.1-1
```

The [GitHub Action page](docs/public/github-action.md) shows how to
[pin the commit SHA](docs/public/github-action.md#pinning) of a release tag
instead, lists every input and output, and documents the exported environment
contract, runner requirements and diagnostics.

## Go deeper

| Topic | Page | Read this when you want to... |
|---|---|---|
| Local quickstart | [quickstart.md](docs/public/quickstart.md) | Boot a LocalNet locally and learn the ports, optional layers, remote tunnel and multi-synchronizer profile |
| GitHub Action | [github-action.md](docs/public/github-action.md) | Run a LocalNet in your own CI and read the exported endpoints and credentials |
| Integration testing | [integration-testing.md](docs/public/integration-testing.md) | Attach the C# or Go fixture to a LocalNet and wire it into CI |
| Topologies | [topologies.md](docs/public/topologies.md) | Choose which slots, PQS and observability layers to run |
| Config schema | [canton-localnet-yaml-schema.md](docs/public/canton-localnet-yaml-schema.md) | Configure the stack with `canton-localnet.yaml` |
| Scripting | [scripting.md](docs/public/scripting.md) | Mint tokens and read endpoints from a script with `auth token` and `info` |
| Funding a party | [funding-amulet.md](docs/public/funding-amulet.md) | Give a validator party an Amulet balance with `tap` |
| Pruning rights | [pruning-rights.md](docs/public/pruning-rights.md) | Audit and sweep leaked ledger rights on a long-lived LocalNet |
| Onboarding clients | [onboarding-clients.md](docs/public/onboarding-clients.md) | Create users in a validator realm without the master admin |
| Migration | [MIGRATION.md](docs/public/MIGRATION.md) | Move an existing setup onto the current release |

## Project stewardship

`canton-localnet` is currently developed and maintained by **Peaceful Studio
OÜ** (Estonia). The project is licensed under Apache-2.0
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
