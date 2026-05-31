# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

output "role_arn" {
  value       = aws_iam_role.ci_state.arn
  description = "Set this as the AWS_STATE_ROLE_ARN GitHub Actions secret."
}

output "oidc_provider_arn" {
  value       = aws_iam_openid_connect_provider.github.arn
  description = "Account-wide GitHub Actions OIDC provider ARN (shareable by other repos/roles)."
}
