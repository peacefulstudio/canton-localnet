#!/bin/bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -eo pipefail

export D_VALIDATOR_1_VALIDATOR_USER_TOKEN=$(curl -fsS "${AUTH_D_VALIDATOR_1_TOKEN_URL}" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=${AUTH_D_VALIDATOR_1_VALIDATOR_CLIENT_ID}" \
  -d 'client_secret='${AUTH_D_VALIDATOR_1_VALIDATOR_CLIENT_SECRET} \
  -d "grant_type=client_credentials" \
  -d "scope=openid" | tr -d '\n' | grep -o -E '"access_token"[[:space:]]*:[[:space:]]*"[^"]+' | grep -o -E '[^"]+$')
