# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

variable "github_repo" {
  type        = string
  default     = "peacefulstudio/canton-localnet-internal"
  description = "owner/repo whose GitHub Actions workflows may assume the CI role via OIDC."
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
  default     = "cicd-playground-tfstate"
  description = "S3 bucket holding Terraform state."
}

variable "state_key_prefix" {
  type        = string
  default     = "canton-localnet/hetzner"
  description = "Key prefix the CI role may read/write: the Hetzner state object and its S3-native lockfile."
}
