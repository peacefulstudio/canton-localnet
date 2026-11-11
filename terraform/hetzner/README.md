<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Hetzner Cloud LocalNet VM

A part-time Canton LocalNet on a Hetzner Cloud CCX33 server. The server is
deleted at night and recreated in the morning to keep compute costs near
business-hours only. LocalNet **data persists across the nightly cycle**: it
lives on a dedicated `hcloud_volume` that is never destroyed by the down/up
toggle. Only the server and its volume attachment are torn down on `down`; the
volume, primary IP, SSH key, and firewall survive, and the next `up` re-attaches
the existing volume. Onboarded parties, DARs, and ledger state therefore carry
over from one day to the next.

## What the volume holds

The volume mounts at `/mnt/canton-localnet-data` and holds both the git checkout
(`/mnt/canton-localnet-data/canton-localnet`) and Docker's data-root
(`/mnt/canton-localnet-data/docker`, configured in `/etc/docker/daemon.json` at
provisioning). Images, named volumes, and container state are therefore all on
the persistent tier.

Provisioning brings LocalNet up through `scripts/bring-up-localnet.sh`, which
decides whether the docker volumes survive. After every successful `make up` it
records a state key in `/mnt/canton-localnet-data/localnet-state-key`:
`SPLICE_VERSION` (which pins the Canton image), `POSTGRES_VERSION` and
`CANTON_PROTOCOL_VERSION`. On the next boot it compares that key with the one the
checkout now resolves:

- **Unchanged:** `make down`, which resets containers and keeps the volumes, so
  ledger state carries over.
- **Changed or missing:** `make clean` (down with volumes), then `make up` on
  fresh volumes. A failed `make clean` aborts provisioning instead of starting
  the new version on the old data.

The key is written only after `make up` succeeds, so a failed bring-up wipes
again on the next boot. The decision and both keys are logged to
`/var/log/canton-localnet-bootstrap.log`. To force a wipe without a version
bump, delete the key file and re-provision, or run `make clean` on the server:

```bash
cd /mnt/canton-localnet-data/canton-localnet && make clean
```

## The checkout

The box runs a published release of the public twin
`peacefulstudio/canton-localnet`, cloned without a token. On every provision
cloud-init clones `repo_url` when the volume holds no checkout, sets the
checkout's `origin` to `repo_url` (a volume first cloned from another
repository fetches the twin from then on), fetches with `--tags --force`, and
checks out `repo_ref`. A tag checkout leaves HEAD detached, so the closing
`git pull --ff-only` runs only when `repo_ref` is a branch. The scheduled
workflow resolves the newest published twin release and passes it as
`repo_ref`; a `workflow_dispatch` `repo_ref` input overrides it.

## Why delete instead of power off?

On Hetzner a powered-off server still bills at the full hourly rate. To actually
save money the server must be deleted, not stopped. Part-time operation is
implemented as delete + recreate by toggling `server_enabled`, never as a
power-off.

## Requirements

- Terraform `>= 1.6`
- A Hetzner Cloud API token, supplied via `TF_VAR_hcloud_token`.
- AWS credentials with access to the S3 state backend (see below).

## State backend

Remote state lives in S3 via [partial
configuration](https://developer.hashicorp.com/terraform/language/backend#partial-configuration):
`versions.tf` declares an empty `backend "s3" {}`. Copy `backend.hcl.example` to
`backend.hcl` (gitignored), set your own bucket, then run `terraform init
-backend-config=backend.hcl`:

| Setting | Value |
|---------|-------|
| bucket | `REPLACE_WITH_YOUR_TFSTATE_BUCKET` |
| key | `canton-localnet/hetzner/terraform.tfstate` |
| region | `eu-north-1` |
| encryption | enabled |
| locking | `use_lockfile` (S3-native lockfile) |

## Host hardening

Cloud-init applies the pending OS updates on first boot and enables
`unattended-upgrades`. It installs Docker from Docker's apt repository only after
the downloaded signing key's fingerprint equals the published
`9DC858229FC7DD38854AE2D88D81803C0EBFCD88`, at the versions in `docker_versions`
(`docker-ce` and `docker-ce-cli` share `engine`), and holds those packages so a
system upgrade cannot move them. To bump Docker, change `docker_versions` and let
the next nightly recreate pick it up.

The volume is formatted by `scripts/format-blank-volume.sh`, and only when
`blkid -p` reports no `TYPE` and the first MiB is all zero. Any other `blkid`
outcome, or data in the first MiB, aborts provisioning without formatting, so a
reattached ledger volume is never reformatted on a doubtful probe.

After LocalNet comes up, provisioning runs `make wait-ready`, which polls the
JSON Ledger API of every enabled slot (sv, a and b on the box, c and d being off),
and only then marks the box provisioned.

## SSH access

There is no generated SSH key. Access is controlled via the
`developer_ssh_public_keys` variable: supply a map of name → public-key string
and each entry is registered as an `hcloud_ssh_key` named
`canton-localnet-<key>`. An empty map (the default) means no developer keys are
registered.

`ssh_allowed_cidrs` stays open to `0.0.0.0/0`, key-only. Narrowing it was
considered and rejected: the consumer SSH lanes (terraform-provider-canton `ci.yaml`
and others) run on GitHub-hosted
`ubuntu-latest` runners, whose IPs cannot be enumerated, so any allowlist breaks
them. Login is key-only and the box holds no secret. Revisit if those lanes move
to `[self-hosted, hetzner]`.

Cloud-init writes `/etc/ssh/sshd_config.d/10-hardening.conf` (password and
keyboard-interactive login off, `PermitRootLogin prohibit-password`, public-key
authentication only, three attempts, a 30 s login grace time, no X11 forwarding),
validates it with `sshd -t` and reloads `ssh`. It restricts neither who may log in
nor from where, and keeps TCP forwarding for `canton-localnet tunnel`.

## Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `server_enabled` | `true` | Whether the server runs. `false` deletes the server and its volume attachment while keeping the volume, primary IP, SSH key, and firewall. |
| `location` | `hel1` | Hetzner location for the server, volume, and persistent primary IP. |
| `volume_size` | `100` | Persistent LocalNet data volume size, in GB. |
| `server_type` | `ccx33` | Hetzner server type (CCX33 = 8 dedicated vCPU, 32 GB RAM). |
| `image` | `ubuntu-24.04` | Base OS image. |
| `hcloud_token` | — (sensitive) | Hetzner Cloud API token. Provide via `TF_VAR_hcloud_token`. |
| `ssh_allowed_cidrs` | `["0.0.0.0/0"]` | CIDRs allowed to reach SSH (port 22). Narrow this to harden access. |
| `repo_url` | `https://github.com/peacefulstudio/canton-localnet.git` | Public git repository the server clones and fetches anonymously for LocalNet compose assets. Cloud-init points the checkout's `origin` here on every provision. |
| `repo_ref` | `null` (required when `server_enabled` is true) | Git ref (release tag, branch, or SHA) of `repo_url` to check out. The scheduled workflow passes the newest published release; a dispatch input overrides it for one run. |
| `docker_versions` | `engine` `5:29.9.3-1`, `containerd` `2.4.2-3`, `buildx` `0.38.1-1`, `compose` `5.7.0-1` | Pinned Docker apt package versions without the distro suffix, installed from Docker's repository and held. |
| `developer_ssh_public_keys` | `{}` | Map of name → SSH public-key string. Each entry is registered as an `hcloud_ssh_key` named `canton-localnet-<key>`. |

## Outputs

| Output | Description |
|--------|-------------|
| `elastic_ip` | Persistent public IPv4 address, stable across down/up cycles. |

## Lifecycle

```bash
make hetzner-up      # terraform apply — create server, attach existing volume
make hetzner-down    # terraform apply -var server_enabled=false — delete server only
```

The two targets wrap raw Terraform; the equivalents are:

```bash
terraform -chdir=terraform/hetzner apply
terraform -chdir=terraform/hetzner apply -var server_enabled=false
```

To remove **everything**, including the persistent volume, primary IP, SSH key,
and firewall:

The `hcloud_volume` and `hcloud_primary_ip` resources carry a
`lifecycle { prevent_destroy = true }` guard so a routine `down`/`up` (or an
accidental `destroy`) can never wipe ledger data. A plain
`terraform -chdir=terraform/hetzner destroy` therefore **fails** on those two
resources by design. Intentional teardown is a deliberate two-step: first remove
the `prevent_destroy` lifecycle blocks from `main.tf`, then run:

```bash
terraform -chdir=terraform/hetzner destroy
```

## Remote tests

```bash
canton-localnet tunnel --host "$(terraform -chdir=terraform/hetzner output -raw elastic_ip)"   # forwards the host-side *975 JSON-API ports; --user defaults to root
export CANTON_LOCALNET_HOST=localhost
go test ./...                      # or the dotnet test suite
```
