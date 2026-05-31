# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

locals {
  repo_owner = split("/", var.github_repo)[0]
  repo_name  = split("/", var.github_repo)[1]
}

provider "github" {
  owner = local.repo_owner
}

data "tls_certificate" "github" {
  url = "https://token.actions.githubusercontent.com/.well-known/openid-configuration"
}

resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github.certificates[0].sha1_fingerprint]
}

data "aws_iam_policy_document" "trust" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repo}:environment:${var.github_environment}"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:ref"
      values   = ["refs/heads/${var.deploy_branch}"]
    }
  }
}

resource "aws_iam_role" "ci_state" {
  name               = "canton-localnet-ci-tfstate"
  assume_role_policy = data.aws_iam_policy_document.trust.json
  description        = "GitHub Actions OIDC role for the Hetzner LocalNet scheduler to read/write its Terraform state."
}

data "aws_iam_policy_document" "state_access" {
  statement {
    sid       = "ListStateBucketPrefix"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::${var.state_bucket}"]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${var.state_key_prefix}/*"]
    }
  }

  statement {
    sid       = "ReadWriteStateObjects"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["arn:aws:s3:::${var.state_bucket}/${var.state_key_prefix}/*"]
  }
}

resource "aws_iam_role_policy" "state_access" {
  name   = "tfstate-access"
  role   = aws_iam_role.ci_state.id
  policy = data.aws_iam_policy_document.state_access.json
}

resource "github_repository_environment" "infra" {
  repository  = local.repo_name
  environment = var.github_environment

  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "deploy_branch_only" {
  repository     = local.repo_name
  environment    = github_repository_environment.infra.environment
  branch_pattern = var.deploy_branch
}
