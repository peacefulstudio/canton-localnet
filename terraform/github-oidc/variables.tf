# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

variable "github_repo" {
  type        = string
  nullable    = false
  description = "owner/repo whose GitHub Actions workflows may assume the CI role via OIDC. Required, with no default: it must be the repository whose hetzner-localnet-schedule workflow runs, and a default naming any other repository would silently re-point the trust policy there on the next apply."

  validation {
    condition     = can(regex("^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$", var.github_repo))
    error_message = "github_repo must be owner/repo, with only [A-Za-z0-9._-] on each side: it is interpolated into the OIDC sub claim the IAM trust policy matches, so a wildcard or an empty side is refused."
  }
}

variable "github_environment" {
  type        = string
  default     = "localnet-infra"
  description = "GitHub Actions deployment environment the scheduler job runs in; the trust policy requires this in the OIDC sub claim."
}

variable "deploy_branch" {
  type        = string
  default     = "dev"
  description = "Branch the scheduler may assume the role from; enforced in IAM via the OIDC ref claim and in GitHub via the environment deployment-branch policy."
}

variable "state_bucket" {
  type        = string
  default     = "REPLACE_WITH_YOUR_TFSTATE_BUCKET"
  description = "S3 bucket holding Terraform state."
}

variable "state_key_prefix" {
  type        = string
  default     = "canton-localnet/hetzner"
  description = "Key prefix the CI role may read/write: the Hetzner state object and its S3-native lockfile."
}
