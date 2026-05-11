<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Canton LocalNet VM — Terraform

Provisions the persistent spot EC2 instance that hosts Canton LocalNet for
consumer repos (currently `murmures`, future tenants pluggable via the
`consumer_repo` variable).

This configuration was migrated from `peacefulstudio/murmures` —
`infra/terraform/`. It targets the **same** EC2 instance / SG / EIP /
key pair that murmures created. The first run after merge is a one-time
**state migration** captured below; nobody re-creates the VM.

## Layout

```
terraform/
  backend.tf            # S3 backend, key: canton-localnet/vm/terraform.tfstate
  main.tf               # aws_instance / aws_security_group / aws_eip / aws_key_pair
  variables.tf          # region, instance_type, project_name, github_token, ...
  outputs.tf            # instance_id, elastic_ip, ssh_command, ssh_key_path, ...
  templates/
    user_data.sh.tftpl  # cloud-init that clones the consumer repo + runs deploy.sh
  iam-policy.json       # IAM policy to attach to the CI principal (HITL step 2)
  README.md
```

## Variables

| Name              | Default                       | Notes                                                                                |
| ----------------- | ----------------------------- | ------------------------------------------------------------------------------------ |
| `region`          | `eu-north-1`                  | Must match the backend bucket region                                                 |
| `instance_type`   | `m6i.2xlarge`                 | 8 vCPU / 32 GB RAM                                                                   |
| `volume_size`     | `35`                          | GB, gp3                                                                              |
| `project_name`    | `murmures-localnet`           | Used for `Name` tag, SG name, key pair name. Kept verbatim so state import is clean. |
| `consumer_repo`   | `peacefulstudio/murmures`     | Clone target: URL is `https://github.com/<consumer_repo>.git`, dir is `$HOME/<basename>`. Must expose `infra/provision/install.sh` + `deploy.sh`. |
| `github_token`    | (required, sensitive)         | PAT used by user_data to clone the consumer repo                                     |
| `localnet_branch` | `dev`                         | Branch the VM clones on first boot                                                   |

## Outputs

`instance_id`, `elastic_ip`, `ssh_command`, `ssh_key_path`, `region`,
`ami_id` — names match murmures' outputs so consumer tunnel scripts work
unchanged after switching their `terraform output -raw …` source.

## Backend

```hcl
bucket = "cicd-playground-tfstate"
key    = "canton-localnet/vm/terraform.tfstate"
region = "eu-north-1"
```

The bucket is shared with murmures and other Peaceful Studio repos. The
**key** is new — this repo writes a fresh state file at
`canton-localnet/vm/terraform.tfstate`, leaving murmures'
`murmures/localnet/terraform.tfstate` untouched. After migration completes
and consumers have moved over, the old murmures key can be archived.

## One-time state migration (HITL — maintainer runs once)

Prerequisites:
- AWS credentials with read/write on `s3://cicd-playground-tfstate/canton-localnet/vm/*`
  and `ec2:*` on the existing murmures-tagged resources.
- The murmures repo handy, in case rollback is needed.
- `terraform >= 1.6`.

### 1. Identify the resources to import

From a checkout of murmures with its state initialized:

```bash
cd murmures/infra/terraform
terraform init
terraform state list
```

Expected output (resource addresses, not IDs):

```
data.aws_ami.ubuntu
data.aws_vpc.default
aws_eip.localnet
aws_instance.localnet
aws_key_pair.localnet
aws_security_group.localnet
local_file.private_key
tls_private_key.localnet
```

Capture the concrete IDs:

```bash
INSTANCE_ID=$(terraform output -raw instance_id)
EIP_ALLOC_ID=$(terraform state show aws_eip.localnet | awk '/^ *id *=/ {gsub(/"/,""); print $3; exit}')
SG_ID=$(terraform state show aws_security_group.localnet | awk '/^ *id *=/ {gsub(/"/,""); print $3; exit}')
KEY_NAME=$(terraform state show aws_key_pair.localnet | awk '/^ *key_name *=/ {gsub(/"/,""); print $3; exit}')
```

Also extract the existing private key so the new state's `local_file`
and `tls_private_key` line up (these are computed locally — see
"Rollback / caveats" below):

```bash
cp .localnet-key.pem /tmp/localnet-key.pem.backup
```

### 2. Init the new state at the canton-localnet key

```bash
cd canton-localnet/terraform
export TF_VAR_github_token=ghp_…   # same PAT murmures uses
terraform init
```

This creates a fresh state object at
`s3://cicd-playground-tfstate/canton-localnet/vm/terraform.tfstate`.

### 3. Import the AWS resources

```bash
terraform import aws_security_group.localnet "$SG_ID"
terraform import aws_key_pair.localnet       "$KEY_NAME"
terraform import aws_instance.localnet       "$INSTANCE_ID"
terraform import aws_eip.localnet            "$EIP_ALLOC_ID"
```

The `tls_private_key.localnet` and `local_file.private_key` resources
are not importable cleanly (the private-key material is generated, not
managed in AWS). Two options:

- **Recommended**: copy the murmures terraform state's
  `tls_private_key.localnet` block into the new state with
  `terraform state mv` from a temporary state file, OR
- **Acceptable**: let terraform regenerate a new key pair on the next
  apply. This rotates the SSH key — consumers must pull the new
  `ssh_command` output. The EC2 instance itself is untouched (the AWS
  `aws_key_pair` resource is already imported above and references the
  existing public key on the instance).

Document which path was taken in the migration runbook.

### 4. Verify plan is clean

```bash
terraform plan
```

**Expected**: `No changes. Your infrastructure matches the configuration.`

**Acceptable diffs** that the maintainer applies as part of migration:

1. `Project` tag value change from `murmures` → `canton-localnet` on
   `aws_instance.localnet`, `aws_security_group.localnet`,
   `aws_eip.localnet`. These are in-place updates, not replacements.
2. The `tls_private_key` / `local_file` pair if option 2 above was
   taken (SSH key rotation only — the EC2 instance is not touched).

**Unacceptable diffs** that must be investigated before applying:

- Any `# forces replacement` annotation on `aws_instance.localnet`. The
  most likely cause is a user_data hash mismatch — confirm
  `terraform/templates/user_data.sh.tftpl` is byte-identical to the
  murmures version (CRLF vs LF, trailing whitespace).
- Any change to `aws_security_group` ingress / egress rules.

If the plan is unacceptable, do not apply — see Rollback below.

### 5. Apply

```bash
terraform apply
```

### 6. Verify

```bash
terraform output ssh_command
ssh -i "$(terraform output -raw ssh_key_path)" "ubuntu@$(terraform output -raw elastic_ip)" hostname
```

### 7. Switch consumers off the old state

Update murmures' tunnel scripts (`murmures/infra/scripts/tunnel*.sh`)
to read outputs from this repo's terraform directory, then archive
murmures' `infra/terraform/` (or delete the resources from its state
without destroying — `terraform state rm` for each).

## Rollback

If the plan after step 4 is unacceptable:

1. Do NOT run `terraform apply`.
2. Remove the imported resources from the new state:
   ```bash
   terraform state rm aws_eip.localnet aws_instance.localnet \
     aws_security_group.localnet aws_key_pair.localnet
   ```
3. Delete the state object:
   ```bash
   aws s3 rm s3://cicd-playground-tfstate/canton-localnet/vm/terraform.tfstate
   ```
4. Murmures' state is unchanged throughout — consumers keep running
   off murmures' terraform until the canton-localnet config is fixed
   and re-imported.

## IAM policy for CI

`iam-policy.json` is the policy the canton-localnet GitHub Actions OIDC
role needs. Scope is constrained by `aws:ResourceTag/Project =
canton-localnet`, so the policy cannot mutate murmures-tagged
resources. Attach it to the canton-localnet CI role after the migration
completes — see the PR body for the human checklist.

## CI

`.github/workflows/terraform-ci.yaml` runs `terraform fmt -check
-recursive` and `terraform validate` (via `init -backend=false`) on every
PR touching `terraform/**`. No `terraform plan` runs in CI yet — that
arrives once the IAM grant in step 7 of the migration lands.
