<!-- Copyright (c) 2026 Peaceful Studio OÜ -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# GitHub Actions OIDC role for Terraform state

Bootstrap config that lets the `hetzner-localnet-schedule` workflow reach the
Terraform state bucket **without long-lived AWS keys**. It creates:

- the account-wide GitHub Actions OIDC provider
  (`token.actions.githubusercontent.com`), and
- an IAM role (`canton-localnet-ci-tfstate`) that this repo's workflows may
  assume, scoped to read/write **only** the Hetzner state object and its
  S3-native lockfile under `canton-localnet/hetzner/`.

This is intentionally a separate config from `terraform/hetzner`: the OIDC
provider is account-global and the role that *grants* state access should not
live inside the state it manages. It keeps its own state at
`canton-localnet/github-oidc/terraform.tfstate`.

## Apply (one-time bootstrap, run with admin AWS credentials)

This config also manages the GitHub `localnet-infra` deployment environment, so
it needs a GitHub token with admin rights on the repo in addition to admin AWS
credentials:

```bash
export GITHUB_TOKEN="$(gh auth token)"
terraform -chdir=terraform/github-oidc init
terraform -chdir=terraform/github-oidc apply
```

Then publish the role ARN to the scheduler workflow:

```bash
gh secret set AWS_STATE_ROLE_ARN -R peacefulstudio/canton-localnet \
  --body "$(terraform -chdir=terraform/github-oidc output -raw role_arn)"
```

## Scope

The role can only be assumed by the `hetzner-localnet-schedule` workflow running
on the `dev` branch through the `localnet-infra` environment. Three independent
gates enforce this:

- **IAM `sub` claim** must equal
  `repo:peacefulstudio/canton-localnet:environment:localnet-infra` —
  the job must run through the `localnet-infra` environment.
- **IAM `ref` claim** must equal `refs/heads/dev` — the run must be on `dev`.
  (`StringEquals` is fail-closed: a missing/absent claim denies the assume.)
- **GitHub environment deployment-branch policy** restricts `localnet-infra` to
  `dev`, so a token for this environment cannot be minted off any other branch.

The IAM `ref` condition relies on AWS STS provider-specific claim validation for
GitHub (GA early 2026). The environment branch policy is the independent
GitHub-side gate; together they enforce the branch restriction on both sides.
