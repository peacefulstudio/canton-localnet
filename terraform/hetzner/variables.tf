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
  default     = "https://github.com/peacefulstudio/canton-localnet.git"
  description = "Public clone URL of the repo whose docker-compose LocalNet stack the box runs. Cloud-init points the checkout's origin here on every provision and fetches without credentials."

  validation {
    condition     = can(regex("^https://[A-Za-z0-9._/-]+$", var.repo_url))
    error_message = "repo_url must be an https:// URL containing only [A-Za-z0-9._/-] (it is inlined into a shell script at boot)."
  }
}

variable "repo_ref" {
  type        = string
  default     = null
  description = "Git ref (release tag, branch or SHA) of repo_url to check out on the box. Required, with no default to fall back to, whenever server_enabled is true; may be omitted when server_enabled is false because no server consumes it."

  validation {
    condition     = var.repo_ref == null ? !var.server_enabled : can(regex("^[A-Za-z0-9._][A-Za-z0-9._/-]*$", var.repo_ref))
    error_message = "repo_ref is required when server_enabled is true, and must contain only [A-Za-z0-9._/-] and cannot start with '-' or '/' (the value is inlined into shell + git commands)."
  }
}

variable "developer_ssh_public_keys" {
  type        = map(string)
  default     = {}
  description = "Map of name → OpenSSH public key to register on the server's authorized_keys. Defaults to empty (no developer SSH access). Pass via TF_VAR_developer_ssh_public_keys or a .tfvars file kept outside the repo."
}

variable "docker_versions" {
  type = object({
    engine     = string
    containerd = string
    buildx     = string
    compose    = string
  })
  default = {
    engine     = "5:29.9.3-1"
    containerd = "2.4.2-3"
    buildx     = "0.38.1-1"
    compose    = "5.7.0-1"
  }
  description = "Pinned Docker apt package versions the box installs, without the distro suffix: engine covers docker-ce and docker-ce-cli, containerd covers containerd.io, buildx covers docker-buildx-plugin and compose covers docker-compose-plugin. Cloud-init appends ~ubuntu.<VERSION_ID>~<VERSION_CODENAME> and holds the packages."

  validation {
    condition     = alltrue([for version in values(var.docker_versions) : can(regex("^[A-Za-z0-9.:+_-]+$", version))])
    error_message = "docker_versions entries must contain only [A-Za-z0-9.:+_-] (they are inlined into a shell script at boot)."
  }
}
