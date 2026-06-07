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
  description = "Name prefix for AWS resources (key pair, security group, Name tag)."
  type        = string
  default     = "canton-localnet"
}

variable "repo_url" {
  description = "Clone URL of the repo whose docker-compose LocalNet stack the box runs. Override with a public mirror to clone without a token."
  type        = string
  default     = "https://github.com/peacefulstudio/canton-localnet.git"

  validation {
    condition     = can(regex("^https://[A-Za-z0-9._/-]+$", var.repo_url))
    error_message = "repo_url must be an https:// URL containing only [A-Za-z0-9._/-] (it is inlined into a shell script at boot)."
  }
}

variable "repo_ref" {
  description = "Git ref (branch or tag) of repo_url to check out on the box."
  type        = string
  default     = "dev"

  validation {
    condition     = can(regex("^[A-Za-z0-9._][A-Za-z0-9._/-]*$", var.repo_ref))
    error_message = "repo_ref must contain only [A-Za-z0-9._/-] and cannot start with '-' or '/' (the value is inlined into shell + git commands)."
  }
}

variable "repo_token" {
  description = "Optional read-only token for cloning a private repo_url. Leave empty when repo_url is a public mirror."
  type        = string
  default     = ""
  sensitive   = true
}
