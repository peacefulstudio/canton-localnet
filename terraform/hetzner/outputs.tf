# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

output "elastic_ip" {
  description = "Stable public IPv4 of the LocalNet VM (survives server recreate)."
  value       = hcloud_primary_ip.localnet.ip_address
}
