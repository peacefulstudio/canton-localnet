# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

provider "aws" {
  region                      = "eu-north-1"
  access_key                  = "mock"
  secret_key                  = "mock"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}

mock_provider "github" {}

mock_provider "tls" {
  mock_data "tls_certificate" {
    defaults = {
      certificates = [
        {
          sha1_fingerprint = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
        }
      ]
    }
  }
}

run "oidc_security_boundary" {
  command = apply

  override_resource {
    target = aws_iam_openid_connect_provider.github
    values = {
      arn = "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com"
    }
  }

  override_resource {
    target = aws_iam_role.ci_state
    values = {
      arn = "arn:aws:iam::123456789012:role/canton-localnet-ci-tfstate"
    }
  }

  override_resource {
    target = aws_iam_role_policy.state_access
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.trust.json).Statement[0].Action == "sts:AssumeRoleWithWebIdentity"
    error_message = "trust statement action must be sts:AssumeRoleWithWebIdentity"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.trust.json).Statement[0].Effect == "Allow"
    error_message = "trust statement effect must be Allow"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.trust.json).Statement[0].Condition.StringEquals["token.actions.githubusercontent.com:aud"] == "sts.amazonaws.com"
    error_message = "the aud condition must equal sts.amazonaws.com"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.trust.json).Statement[0].Condition.StringEquals["token.actions.githubusercontent.com:sub"] == "repo:peacefulstudio/canton-localnet-internal:environment:localnet-infra"
    error_message = "the sub condition must scope to the localnet-infra environment of this repo"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.trust.json).Statement[0].Condition.StringEquals["token.actions.githubusercontent.com:ref"] == "refs/heads/dev"
    error_message = "the ref condition must scope to refs/heads/dev"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.state_access.json).Statement[1].Resource == "arn:aws:s3:::cicd-playground-tfstate/canton-localnet/hetzner/*"
    error_message = "state object access must be scoped to the Hetzner key prefix, never a bare wildcard"
  }

  assert {
    condition     = jsondecode(data.aws_iam_policy_document.state_access.json).Statement[0].Resource == "arn:aws:s3:::cicd-playground-tfstate"
    error_message = "ListBucket must target the state bucket"
  }

  assert {
    condition     = github_repository_environment_deployment_policy.deploy_branch_only.branch_pattern == var.deploy_branch
    error_message = "the GitHub deployment-branch-policy pattern must equal var.deploy_branch"
  }

  assert {
    condition     = var.deploy_branch == "dev"
    error_message = "deploy_branch default must be dev"
  }
}
