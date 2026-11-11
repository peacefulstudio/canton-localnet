<!-- Copyright 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Local quickstart

Boot a complete Canton LocalNet (Splice 0.10.1) on your machine:

```bash
make up           # docker compose up -d, OAuth2 mode by default
make wait-ready   # poll the JSON Ledger API of every enabled slot until each accepts requests
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

Every host-published port (participant ledger / admin / JSON, Splice
validator admin, Postgres `5432`, Keycloak `8082`, swagger-ui, the OTLP
collector `14001` / `14003` and Grafana `3030`) binds to `HOST_BIND_IP`, which
defaults to `127.0.0.1`. Set `HOST_BIND_IP=0.0.0.0` (e.g. on a Hetzner box from
`terraform/hetzner`, whose cloud firewall is SSH-only) to publish on all interfaces:

```bash
make up HOST_BIND_IP=0.0.0.0
```

The demo credentials are public (see below), so publish on `0.0.0.0` only behind
a firewall that admits nobody you do not trust.

Demo credentials are public. The stack ships fixed credentials that are committed to this repository: the Keycloak admin console login (`admin` / `admin`), the Postgres superuser (`DB_USER` / `DB_PASSWORD`, default `cnadmin` and a published password), the OAuth2 client secrets of the imported Keycloak realms (also the built-in defaults of the .NET fixture), and the wallet users' passwords. Anyone who can reach a port can use them, so run the stack on a loopback-only or firewalled host and never against real assets.

On Docker server versions below 28.0.0, a port published to `127.0.0.1` can
still be reachable from other hosts on the same network segment — see
`make check-docker-version` (`STRICT=true` turns that into a hard failure).

Optional layers via Make flags (shortcuts — the canonical config layer is
`canton-localnet.yaml`, see the [config schema](canton-localnet-yaml-schema.md)):

```bash
make up PQS=true            # opt in to PQS for a, b and c, as the CLI --pqs does; a slot whose <SLOT>_VALIDATOR_1_PROFILE is off is skipped
make up PQS_SLOTS=a,c       # PQS for the listed slots (a, b, c, sv); see topologies.md
make up OBS=true            # add Grafana (http://localhost:3030) + Prometheus / Loki / Tempo / cAdvisor; host apps export OTLP to localhost:14001 (gRPC) or localhost:14003 (HTTP)
make up RES=false           # remove the default mem_limit / JVM heap caps
make up AUTH_MODE=secret    # shared-secret JWT (escape hatch; not CI-tested)
make up UP_TIMEOUT=15m      # fail if compose has not returned in 15m; SIGKILL after UP_KILL_AFTER (default 30s) if it ignores the SIGTERM (needs coreutils timeout)
```

When iterating with observability on, `make stop-app` (and `make clean-app`)
restart the application stack while leaving Grafana / Prometheus / Loki / Tempo
running so dashboards stay populated.

## Tunnel to a remote LocalNet

To reach a LocalNet running on another host (a shared VM, a cloud box), open an
`ssh -L` tunnel so its ports appear on `localhost`. The command needs no
repository checkout and no terraform:

```bash
canton-localnet tunnel --host <host>   # --user defaults to root; Ctrl-C closes the tunnel
```

It forwards the host-side JSON Ledger API port of every slot (`10975`-`14975`),
plus `11901` (a-validator-1 participant gRPC ledger API), `7575` (legacy
in-container Splice JSON Ledger API) and `8082` (Keycloak). `--identity` selects
the SSH key (default `~/.ssh/canton-localnet` when that file exists), `--user`
the remote user and `--ssh-port` the remote port. The port set is defined in
[`cli/internal/tunnel`](../../cli/internal/tunnel/tunnel.go).

Prerequisites: Docker ≥ 27, Docker Compose ≥ 2.27. The compose stack is
vendored into `compose/modules/` from `hyperledger-labs/splice` at the SHA
pinned in `compose/splice.sha`. Re-vendor with `make vendor`.

## Multi-synchronizer profile

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

`multi-sync` is local/CI only — it is not enabled on the Hetzner deployment.

