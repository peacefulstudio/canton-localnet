# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.0"
    }
  }
}

provider "aws" {
  region = var.region
}

data "aws_vpc" "default" {
  default = true
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }
}

resource "tls_private_key" "localnet" {
  algorithm = "ED25519"
}

resource "aws_key_pair" "localnet" {
  key_name   = "${var.project_name}-key"
  public_key = tls_private_key.localnet.public_key_openssh
}

resource "local_file" "private_key" {
  content         = tls_private_key.localnet.private_key_openssh
  filename        = "${path.module}/.localnet-key.pem"
  file_permission = "0600"
}

resource "aws_security_group" "localnet" {
  name        = var.project_name
  description = "SSH access for Canton LocalNet"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "SSH (key-authenticated only)"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name    = var.project_name
    Project = "canton-localnet"
  }
}

resource "aws_instance" "localnet" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.localnet.key_name
  vpc_security_group_ids = [aws_security_group.localnet.id]

  user_data = templatefile("${path.module}/templates/user_data.sh.tftpl", {
    github_token    = var.github_token
    localnet_branch = var.localnet_branch
    consumer_repo   = var.consumer_repo
  })
  user_data_replace_on_change = true

  instance_market_options {
    market_type = "spot"
    spot_options {
      spot_instance_type             = "persistent"
      instance_interruption_behavior = "stop"
    }
  }

  root_block_device {
    volume_size           = var.volume_size
    volume_type           = "gp3"
    delete_on_termination = true
  }

  tags = {
    Name    = var.project_name
    Project = "canton-localnet"
  }
}

resource "aws_eip" "localnet" {
  instance = aws_instance.localnet.id
  domain   = "vpc"

  tags = {
    Name    = var.project_name
    Project = "canton-localnet"
  }
}
