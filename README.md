# canton-localnet

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

Shared, reusable Canton LocalNet artifact (compose + xUnit/Go fixtures + CLI + AWS terraform) for Peaceful Studio repos.

## Quickstart

Boot a complete Canton LocalNet (Splice 0.6.2) on your machine:

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

Slot ports follow the 5-digit two-digit-prefix scheme (`<prefix><suffix>`) — see
[ADR-0002](docs/adr/0002-two-digit-port-prefix.md) for the rationale and the
full port table (participant ledger / admin / JSON / Splice validator admin
per slot).

Optional layers:

```bash
make up PQS=on              # opt in to PQS a-validator-1 profile
make up OBS=on              # add Grafana (http://localhost:3030) + Prometheus / Loki / Tempo / cAdvisor
make up RES=off             # remove the default mem_limit / JVM heap caps
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
canton-localnet vm tunnel           # ssh -L 11901/7575/8082 to the VM (Ctrl-C to close)
canton-localnet vm destroy --yes    # terraform destroy (interactive prompt without --yes)
```

The tunnel forwards the same port set the legacy `tunnel.sh` scripts in
`murmures` and `terraform-provider-canton` open. `vm provision` is
idempotent — re-running on an already-applied state is a no-op refresh.

Prerequisites: Docker ≥ 27, Docker Compose ≥ 2.27. The compose stack is
vendored into `compose/modules/` from `hyperledger-labs/splice` at the SHA
pinned in `compose/splice.sha`. Re-vendor with `make vendor`.

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
