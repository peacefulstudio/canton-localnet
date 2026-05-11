# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

output "instance_id" {
  description = "EC2 instance ID"
  value       = aws_instance.localnet.id
}

output "elastic_ip" {
  description = "Elastic IP address (stable across stop/start)"
  value       = aws_eip.localnet.public_ip
}

output "ssh_command" {
  description = "SSH command to connect"
  value       = "ssh -i ${abspath(local_file.private_key.filename)} ubuntu@${aws_eip.localnet.public_ip}"
}

output "ssh_key_path" {
  description = "Path to the SSH private key"
  value       = abspath(local_file.private_key.filename)
}

output "region" {
  description = "AWS region"
  value       = var.region
}

output "ami_id" {
  description = "Ubuntu AMI used"
  value       = data.aws_ami.ubuntu.id
}
