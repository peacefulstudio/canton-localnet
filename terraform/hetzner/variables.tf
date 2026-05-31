# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

variable "server_enabled" {
  type        = bool
  default     = true
  description = "When true, the disposable server (and its volume attachment) are created. Set false to delete only the server while keeping the persistent volume, primary IP, SSH key, and firewall."
}

variable "location" {
  type        = string
  default     = "hel1"
  description = "Hetzner location. hel1 (Finland) is the cheapest tier, runs on green power, and is closest to AWS eu-north-1."
}

variable "volume_size" {
  type        = number
  default     = 100
  description = "Size in GB of the persistent data volume that survives the nightly server delete/recreate cycle."
}

variable "server_type" {
  type        = string
  default     = "ccx33"
  description = "Hetzner server type. ccx33 = 8 dedicated vCPU / 32 GB / 240 GB NVMe."
}

variable "image" {
  type        = string
  default     = "ubuntu-24.04"
  description = "Base OS image for the server."
}

variable "hcloud_token" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Hetzner Cloud API token. Supplied via TF_VAR_hcloud_token in real runs; empty default keeps mocked plan tests credential-free."
}

variable "ssh_allowed_cidrs" {
  type        = list(string)
  default     = ["0.0.0.0/0"]
  description = "CIDRs allowed inbound on SSH (port 22). Defaults to anywhere (key-auth only, matching the AWS posture); narrow to operator/bastion/VPN ranges to harden. IPv6 is disabled on the server, so list IPv4 ranges only."
}

variable "repo_url" {
  type        = string
  default     = "https://github.com/peacefulstudio/canton-localnet-internal.git"
  description = "Clone URL of the repo whose docker-compose LocalNet stack the box runs. Override with a public mirror to clone without a token."

  validation {
    condition     = can(regex("^https://[A-Za-z0-9._/-]+$", var.repo_url))
    error_message = "repo_url must be an https:// URL containing only [A-Za-z0-9._/-] (it is inlined into a shell script at boot)."
  }
}

variable "repo_ref" {
  type        = string
  default     = "dev"
  description = "Git ref (branch or tag) of repo_url to check out on the box."

  validation {
    condition     = can(regex("^[A-Za-z0-9._][A-Za-z0-9._/-]*$", var.repo_ref))
    error_message = "repo_ref must contain only [A-Za-z0-9._/-] and cannot start with '-' or '/' (the value is inlined into shell + git commands)."
  }
}

variable "repo_token" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Optional read-only token for cloning a private repo_url. Leave empty when repo_url is a public mirror."
}
