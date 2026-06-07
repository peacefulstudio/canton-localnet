<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Canton LocalNet VM — Terraform (AWS)

Self-contained AWS Terraform stack that provisions a spot EC2 instance.
On first boot the instance clones this repo (`repo_url` / `repo_ref`) and
runs `make up`, bringing the docker-compose Canton LocalNet stack online.

Cloning the default (private) `repo_url` requires a read-only token
supplied via `TF_VAR_repo_token`. Point `repo_url` at a public mirror to
clone without any token.

## Layout

```
terraform/
  backend.tf            # S3 backend, key: canton-localnet/vm/terraform.tfstate
  main.tf               # aws_instance / aws_security_group / aws_eip / aws_key_pair
  variables.tf          # region, instance_type, volume_size, project_name, repo_url, ...
  outputs.tf            # instance_id, elastic_ip, ssh_command, ssh_key_path, ...
  templates/
    user_data.sh.tftpl  # cloud-init bootstrap that clones this repo + runs `make up`
  iam-policy.json       # IAM policy to attach to the CI principal
  README.md
```

## Variables

| Name            | Default                                                              | Notes                                                          |
| --------------- | ------------------------------------------------------------------- | ------------------------------------------------------------- |
| `region`        | `eu-north-1`                                                        | Must match the backend bucket region                          |
| `instance_type` | `m6i.2xlarge`                                                       | 8 vCPU / 32 GB RAM                                             |
| `volume_size`   | `35`                                                               | GB, gp3                                                        |
| `project_name`  | `canton-localnet`                                                   | Used for `Name` tag, SG name, key pair name                   |
| `repo_url`      | `https://github.com/peacefulstudio/canton-localnet.git`   | Repo the box clones and runs `make up` from. Override with a public mirror to clone tokenless. |
| `repo_ref`      | `dev`                                                              | Git ref (branch or tag) checked out on the box                |
| `repo_token`    | (empty, sensitive)                                                 | Read-only token for cloning a private `repo_url`. Source via `TF_VAR_repo_token`. Leave empty for a public mirror. |

## Outputs

`instance_id`, `elastic_ip`, `ssh_command`, `ssh_key_path`, `region`,
`ami_id`.

## Backend

The S3 backend uses [partial
configuration](https://developer.hashicorp.com/terraform/language/backend#partial-configuration):
`backend.tf` declares an empty `backend "s3" {}` and the bucket is supplied at
init time. Copy `backend.hcl.example` to `backend.hcl` (gitignored), set your
own bucket, then run `terraform init -backend-config=backend.hcl`:

```hcl
bucket = "REPLACE_WITH_YOUR_TFSTATE_BUCKET"
key    = "canton-localnet/vm/terraform.tfstate"
region = "eu-north-1"
```

This stack writes its state at `canton-localnet/vm/terraform.tfstate`.

## IAM policy for CI

`iam-policy.json` is the policy the canton-localnet GitHub Actions OIDC
role needs. Scope is constrained by `aws:ResourceTag/Project =
canton-localnet`.

## CI

`.github/workflows/terraform-ci.yaml` runs `terraform fmt -check
-recursive` and `terraform validate` (via `init -backend=false`) on every
PR touching `terraform/**`.
