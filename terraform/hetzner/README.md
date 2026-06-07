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

## SSH access

There is no generated SSH key. Access is controlled via the
`developer_ssh_public_keys` variable: supply a map of name → public-key string
and each entry is registered as an `hcloud_ssh_key` named
`canton-localnet-<key>`. An empty map (the default) means no developer keys are
registered.

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
| `repo_url` | `…/canton-localnet.git` | Git repository cloned on the server for LocalNet compose assets. |
| `repo_ref` | `dev` | Git ref (branch, tag, or SHA) to check out. |
| `repo_token` | — (sensitive) | Optional token for cloning a private repository. |
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
canton-localnet vm tunnel          # forwards the host-side *975 JSON-API ports by default
export CANTON_LOCALNET_HOST=localhost
go test ./...                      # or the dotnet test suite
```
