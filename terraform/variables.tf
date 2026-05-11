# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

variable "region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "instance_type" {
  description = "EC2 instance type (8 vCPU / 32GB RAM recommended)"
  type        = string
  default     = "m6i.2xlarge"
}

variable "volume_size" {
  description = "EBS root volume size in GB"
  type        = number
  default     = 35
}

variable "project_name" {
  description = "Name prefix for AWS resources (key pair, security group, Name tag). Defaults to the murmures-era prefix so state import lines up byte-for-byte."
  type        = string
  default     = "murmures-localnet"
}

variable "consumer_repo" {
  description = "GitHub repo (owner/name) the VM clones on first boot. The clone URL is derived as https://github.com/<consumer_repo>.git and the clone destination is $HOME/<basename(consumer_repo)>; the repo must expose infra/provision/install.sh and infra/provision/deploy.sh."
  type        = string
  default     = "peacefulstudio/murmures"
}

variable "github_token" {
  description = "GitHub PAT used by the VM bootstrap to clone the consumer repo. Source via TF_VAR_github_token."
  type        = string
  sensitive   = true

  validation {
    condition     = length(trimspace(var.github_token)) > 0
    error_message = "github_token must be a non-empty string. Source it via TF_VAR_github_token."
  }
}

variable "localnet_branch" {
  description = "Git branch the VM clones and deploys on first boot."
  type        = string
  default     = "dev"

  validation {
    condition     = can(regex("^[A-Za-z0-9._][A-Za-z0-9._/-]*$", var.localnet_branch))
    error_message = "localnet_branch must contain only [A-Za-z0-9._/-] and cannot start with '-' or '/' (the value is inlined into shell + git commands)."
  }
}
