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
  repo_ref     = "v0.10.0-1"
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

}

run "repo_ref_rejects_empty" {
  command = plan

  variables {
    repo_ref = ""
  }

  expect_failures = [var.repo_ref]
}

run "repo_ref_required_when_server_enabled" {
  command = plan

  variables {
    server_enabled = true
    repo_ref       = null
  }

  expect_failures = [var.repo_ref]
}

run "repo_ref_optional_when_server_disabled" {
  command = plan

  variables {
    server_enabled = false
    repo_ref       = null
  }

  assert {
    condition     = length(hcloud_server.localnet) == 0
    error_message = "down must plan without a repo_ref and keep the server absent"
  }
}

run "repo_token_variable_is_gone" {
  command = plan

  assert {
    condition     = var.repo_ref != "" && !strcontains(file("variables.tf"), "repo_token")
    error_message = "the box clones the public twin, so the module must not declare a repo_token variable"
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
    condition     = strcontains(hcloud_server.localnet[0].user_data, indent(6, file("scripts/bring-up-localnet.sh")))
    error_message = "cloud-init must ship scripts/bring-up-localnet.sh verbatim, so the box runs the tested wipe-on-version-change logic"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "/opt/canton-localnet/bring-up-localnet.sh \"$SRC\" \"$MOUNT/localnet-state-key\" make up PQS=true")
    error_message = "provisioning must bring LocalNet up through bring-up-localnet.sh, keyed on a state key stored on the persistent volume"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "make -C \"$SRC\" wait-ready\n")
    error_message = "provisioning must wait for every enabled slot to answer readyz before it records the box as provisioned, as the CLI boot does"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "make up PQS=true\n\n      make -C \"$SRC\" wait-ready\n\n      mkdir -p \"$(dirname \"$DONE_FLAG\")\"")
    error_message = "the readiness wait must sit between the bring-up and the provisioned flag, so a stack that never answers leaves the box unflagged"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git clone")
    error_message = "cloud-init must clone the repo"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "/opt/canton-localnet/format-blank-volume.sh \"$DEVICE\"")
    error_message = "the volume must be formatted only through format-blank-volume.sh, so a reattached ledger volume is never reformatted on a doubtful probe (data loss)"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, indent(6, file("scripts/format-blank-volume.sh")))
    error_message = "cloud-init must ship scripts/format-blank-volume.sh verbatim, so the box runs the tested blank-volume guard"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "if ! blkid") && !strcontains(hcloud_server.localnet[0].user_data, "mkfs.ext4 -F \"$DEVICE\"")
    error_message = "provision.sh must not format the volume itself; any blkid failure would then format it"
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

run "cloud_init_clones_the_twin_release_without_a_token" {
  command = apply

  variables {
    server_enabled = true
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git clone \"https://github.com/peacefulstudio/canton-localnet.git\" \"$SRC\"")
    error_message = "cloud-init must clone repo_url with plain git, no credential helper"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "AUTH_HEADER")
    error_message = "user-data must not carry an auth header path"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "extraHeader")
    error_message = "git must not be given an http.extraHeader"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "REPO_TOKEN")
    error_message = "user-data must not mention a repo token"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "embedded credential")
    error_message = "the legacy credential-stripping step is superseded by re-pointing origin"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git -C \"$SRC\" remote set-url origin \"https://github.com/peacefulstudio/canton-localnet.git\"")
    error_message = "cloud-init must point origin at repo_url before fetching, so a volume cloned from the private internal repo fetches the public twin"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git -C \"$SRC\" fetch --prune --tags --force origin")
    error_message = "cloud-init must fetch tags from origin, forcing them because the legacy checkout holds internal tags of the same names"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git -C \"$SRC\" checkout \"v0.10.0-1\"")
    error_message = "cloud-init must check out repo_ref"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "git -C \"$SRC\" pull --ff-only")
    error_message = "cloud-init must keep the fast-forward pull for a branch checkout"
  }
}

run "cloud_init_applies_updates_and_hardens_ssh" {
  command = apply

  variables {
    server_enabled = true
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "package_upgrade: true")
    error_message = "the box must apply the pending OS updates on first boot"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "  - unattended-upgrades\n")
    error_message = "the box must install unattended-upgrades"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "APT::Periodic::Unattended-Upgrade \"1\";")
    error_message = "unattended-upgrades must be switched on, not only installed"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "path: /etc/ssh/sshd_config.d/10-hardening.conf")
    error_message = "sshd hardening must live in a drop-in, not an edit of sshd_config"
  }

  assert {
    condition = alltrue([
      for directive in [
        "PasswordAuthentication no",
        "KbdInteractiveAuthentication no",
        "PermitEmptyPasswords no",
        "PermitRootLogin prohibit-password",
        "PubkeyAuthentication yes",
        "AuthenticationMethods publickey",
        "MaxAuthTries 3",
        "LoginGraceTime 30",
        "X11Forwarding no",
      ] : strcontains(hcloud_server.localnet[0].user_data, "      ${directive}\n")
    ])
    error_message = "the sshd drop-in must keep login key-only for root and cap the attempt window"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "sshd -t && systemctl reload ssh")
    error_message = "the drop-in must be validated with sshd -t before sshd is reloaded, so a typo cannot lock SSH out"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "AllowUsers") && !strcontains(hcloud_server.localnet[0].user_data, "ListenAddress")
    error_message = "SSH stays open to the key-only developers: the drop-in must not restrict who or where from"
  }
}

run "cloud_init_installs_pinned_docker_from_a_verified_key" {
  command = apply

  variables {
    server_enabled = true
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "EXPECTED_DOCKER_KEY_FINGERPRINT=9DC858229FC7DD38854AE2D88D81803C0EBFCD88")
    error_message = "cloud-init must name Docker's published apt signing key fingerprint"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "gpg --show-keys --with-colons")
    error_message = "cloud-init must read the downloaded key's fingerprint before trusting it"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "refusing to trust the Docker apt key")
    error_message = "a fingerprint mismatch must abort provisioning"
  }

  assert {
    condition = alltrue([
      for package in [
        "docker-ce=5:29.9.3-1~ubuntu.$VERSION_ID~$VERSION_CODENAME",
        "docker-ce-cli=5:29.9.3-1~ubuntu.$VERSION_ID~$VERSION_CODENAME",
        "containerd.io=2.4.2-3~ubuntu.$VERSION_ID~$VERSION_CODENAME",
        "docker-buildx-plugin=0.38.1-1~ubuntu.$VERSION_ID~$VERSION_CODENAME",
        "docker-compose-plugin=5.7.0-1~ubuntu.$VERSION_ID~$VERSION_CODENAME",
      ] : strcontains(hcloud_server.localnet[0].user_data, package)
    ])
    error_message = "every Docker package must be installed at the pinned default version"
  }

  assert {
    condition     = strcontains(hcloud_server.localnet[0].user_data, "apt-mark hold docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin")
    error_message = "the pinned Docker packages must be held so a system upgrade cannot move them"
  }

  assert {
    condition     = length(split("apt-get install -y -o Acquire::Retries=3 --allow-downgrades", hcloud_server.localnet[0].user_data)) == 2
    error_message = "the Docker install line must appear exactly once"
  }

  assert {
    condition     = !strcontains(hcloud_server.localnet[0].user_data, "install -y docker-ce docker-ce-cli")
    error_message = "no unpinned Docker install line may remain"
  }
}

run "docker_versions_default_to_the_pinned_release" {
  command = plan

  assert {
    condition     = var.docker_versions.engine == "5:29.9.3-1" && var.docker_versions.containerd == "2.4.2-3" && var.docker_versions.buildx == "0.38.1-1" && var.docker_versions.compose == "5.7.0-1"
    error_message = "docker_versions must default to the pinned Docker release"
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
