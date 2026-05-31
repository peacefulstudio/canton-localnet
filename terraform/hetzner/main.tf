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

resource "tls_private_key" "localnet" {
  algorithm = "ED25519"
}

resource "local_sensitive_file" "private_key" {
  content         = tls_private_key.localnet.private_key_openssh
  filename        = "${path.module}/.localnet-key.pem"
  file_permission = "0600"
}

resource "hcloud_ssh_key" "localnet" {
  name       = "canton-localnet-key"
  public_key = tls_private_key.localnet.public_key_openssh
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
  ssh_keys     = [hcloud_ssh_key.localnet.id]
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
