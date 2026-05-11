# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

terraform {
  backend "s3" {
    bucket       = "cicd-playground-tfstate"
    key          = "canton-localnet/vm/terraform.tfstate"
    region       = "eu-north-1"
    encrypt      = true
    use_lockfile = true
  }
}
