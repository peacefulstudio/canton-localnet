# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

resource "hcloud_volume" "localnet" {
  name              = "canton-localnet-data"
  size              = var.volume_size
  location          = var.location
  format            = "ext4"
  delete_protection = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "hcloud_ssh_key" "developer" {
  for_each   = var.developer_ssh_public_keys
  name       = "canton-localnet-${each.key}"
  public_key = each.value
}

resource "hcloud_primary_ip" "localnet" {
  name              = "canton-localnet-ip"
  type              = "ipv4"
  location          = var.location
  auto_delete       = false
  delete_protection = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "hcloud_firewall" "localnet" {
  name = "canton-localnet-fw"

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.ssh_allowed_cidrs
  }
}

resource "hcloud_server" "localnet" {
  count        = var.server_enabled ? 1 : 0
  name         = "canton-localnet"
  server_type  = var.server_type
  image        = var.image
  location     = var.location
  ssh_keys     = [for k in hcloud_ssh_key.developer : k.id]
  firewall_ids = [hcloud_firewall.localnet.id]

  public_net {
    ipv4_enabled = true
    ipv4         = hcloud_primary_ip.localnet.id
    ipv6_enabled = false
  }

  user_data = templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
    volume_id  = hcloud_volume.localnet.id
    repo_url   = var.repo_url
    repo_ref   = var.repo_ref
    repo_token = var.repo_token
  })
}

resource "hcloud_volume_attachment" "localnet" {
  count     = var.server_enabled ? 1 : 0
  volume_id = hcloud_volume.localnet.id
  server_id = hcloud_server.localnet[0].id
  automount = false
}
