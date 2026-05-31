# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

output "primary_ip" {
  description = "Stable public IPv4 of the LocalNet VM (survives server recreate)."
  value       = hcloud_primary_ip.localnet.ip_address
}

output "ssh_key_path" {
  description = "Path to the generated SSH private key."
  value       = abspath(local_sensitive_file.private_key.filename)
}

output "ssh_command" {
  description = "SSH command to connect to the LocalNet VM."
  value       = "ssh -i ${abspath(local_sensitive_file.private_key.filename)} root@${hcloud_primary_ip.localnet.ip_address}"
}
