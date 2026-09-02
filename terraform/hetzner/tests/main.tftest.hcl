# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

mock_provider "hcloud" {
  mock_resource "hcloud_volume" {
    defaults = {
      id = "100"
    }
  }

  mock_resource "hcloud_firewall" {
    defaults = {
      id = "1"
    }
  }

  mock_resource "hcloud_ssh_key" {
    defaults = {
      id = "2"
    }
  }

  mock_resource "hcloud_primary_ip" {
    defaults = {
      id         = "3"
      ip_address = "203.0.113.10"
    }
  }

  mock_resource "hcloud_server" {
    defaults = {
      id = "4"
    }
  }
}

variables {
  hcloud_token = "mock-token"
  repo_url     = "https://github.com/peacefulstudio/canton-localnet.git"
}

run "variables_resolve" {
  command = plan

  assert {
    condition     = var.location == "hel1"
    error_message = "default location must be hel1"
  }

  assert {
    condition     = var.volume_size == 100
    error_message = "default volume_size must be 100"
  }

  assert {
    condition     = var.server_type == "ccx33"
    error_message = "default server_type must be ccx33"
  }

  assert {
    condition     = var.image == "ubuntu-24.04"
    error_message = "default image must be ubuntu-24.04"
  }

  assert {
    condition     = var.repo_ref == "dev"
    error_message = "repo_ref default must be dev"
  }
}

run "volume_shape" {
  command = plan

  assert {
    condition     = hcloud_volume.localnet.size == var.volume_size
    error_message = "volume size must equal var.volume_size"
  }

  assert {
    condition     = hcloud_volume.localnet.format == "ext4"
    error_message = "volume must be formatted ext4"
  }
}

run "primary_ip_shape" {
  command = plan

  assert {
    condition     = hcloud_primary_ip.localnet.type == "ipv4"
    error_message = "primary ip must be ipv4"
  }

  assert {
    condition     = hcloud_primary_ip.localnet.auto_delete == false
    error_message = "primary ip must be retained when the server is deleted"
  }

  assert {
    condition     = hcloud_primary_ip.localnet.location == var.location
    error_message = "primary ip location must equal var.location"
  }
}

run "firewall_ssh_only" {
  command = plan

  assert {
    condition     = length(hcloud_firewall.localnet.rule) == 1
    error_message = "firewall must have exactly one inbound rule"
  }

  assert {
    condition     = one(hcloud_firewall.localnet.rule).direction == "in"
    error_message = "the rule must be inbound"
  }

  assert {
    condition     = one(hcloud_firewall.localnet.rule).protocol == "tcp"
    error_message = "the rule must be tcp"
  }

  assert {
    condition     = one(hcloud_firewall.localnet.rule).port == "22"
    error_message = "the rule must open port 22"
  }
}

run "server_shape_when_enabled" {
  command = plan

  variables {
    server_enabled = true
  }

  assert {
    condition     = length(hcloud_server.localnet) == 1
    error_message = "server must exist when server_enabled is true"
  }

  assert {
    condition     = hcloud_server.localnet[0].server_type == "ccx33"
    error_message = "server_type must be ccx33"
  }

  assert {
    condition     = hcloud_server.localnet[0].image == "ubuntu-24.04"
    error_message = "image must be ubuntu-24.04"
  }

  assert {
    condition     = hcloud_server.localnet[0].location == var.location
    error_message = "location must equal var.location"
  }
}

run "server_absent_when_disabled" {
  command = plan

  variables {
    server_enabled = false
  }

  assert {
    condition     = length(hcloud_server.localnet) == 0
    error_message = "server must be absent when server_enabled is false"
  }

  assert {
    condition     = hcloud_volume.localnet.size == var.volume_size
    error_message = "volume must remain in plan when server is disabled"
  }

  assert {
    condition     = hcloud_primary_ip.localnet.type == "ipv4"
    error_message = "primary ip must remain in plan when server is disabled"
  }
}

run "attachment_when_enabled" {
  command = plan

  variables {
    server_enabled = true
  }

  assert {
    condition     = length(hcloud_volume_attachment.localnet) == 1
    error_message = "volume attachment must exist when server_enabled is true"
  }

  assert {
    condition     = hcloud_volume_attachment.localnet[0].automount == false
    error_message = "volume attachment must not automount; the provision script owns the mount to avoid a conflicting Hetzner fstab entry"
  }
}

run "attachment_absent_when_disabled" {
  command = plan

  variables {
    server_enabled = false
  }

  assert {
    condition     = length(hcloud_volume_attachment.localnet) == 0
    error_message = "volume attachment must be absent when server_enabled is false"
  }
}

run "cloud_init_renders_mount_and_compose" {
  command = apply

  variables {
    server_enabled = true
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "/dev/disk/by-id/scsi-0HC_Volume_")
    error_message = "cloud-init must mount the Hetzner volume by-id path"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "findmnt \"$MOUNT\" >/dev/null || mount \"$DEVICE\" \"$MOUNT\"")
    error_message = "cloud-init must mount idempotently by device+target, not rely on an fstab name lookup"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "\"data-root\": \"$MOUNT/docker\"")
    error_message = "cloud-init must relocate the Docker data-root onto the persistent volume so ledger state survives the nightly recreate"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "make up")
    error_message = "cloud-init must start LocalNet via the root Makefile (make up)"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "make up PQS=true")
    error_message = "cloud-init must start LocalNet with PQS enabled — a downstream integration lane reads pqs-a-validator-1"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git clone")
    error_message = "cloud-init must clone the repo"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "if ! blkid")
    error_message = "mkfs must stay guarded by a blkid probe so a reattached ledger volume is never reformatted (data loss)"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "never appeared after 120s")
    error_message = "cloud-init must fail fast if the volume device never appears, before any mkfs/mount runs"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "nofail")
    error_message = "the fstab entry must use nofail so a missing volume cannot wedge boot"
  }
}

run "outputs_present" {
  command = plan

  assert {
    condition     = output.elastic_ip == "203.0.113.10"
    error_message = "elastic_ip output must equal the primary IP address"
  }
}

run "developer_keys_empty_by_default" {
  command = plan

  assert {
    condition     = length(hcloud_ssh_key.developer) == 0
    error_message = "no developer SSH keys must be registered when developer_ssh_public_keys is empty"
  }
}

run "developer_keys_present" {
  command = plan

  variables {
    developer_ssh_public_keys = {
      alice = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyForPlanValidationOnlyNotARealKey0000"
    }
  }

  assert {
    condition     = length(hcloud_ssh_key.developer) == 1
    error_message = "one developer SSH key must be registered when developer_ssh_public_keys has one entry"
  }

  assert {
    condition     = hcloud_ssh_key.developer["alice"].name == "canton-localnet-alice"
    error_message = "developer SSH key must be named canton-localnet-<map-key>"
  }
}
