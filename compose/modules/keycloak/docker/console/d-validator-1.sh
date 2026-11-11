#!/bin/bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -eo pipefail

D_VALIDATOR_1_VALIDATOR_USER_TOKEN=$(curl -fsS --max-time 30 "${AUTH_D_VALIDATOR_1_TOKEN_URL}" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "client_id=${AUTH_D_VALIDATOR_1_VALIDATOR_CLIENT_ID}" \
  --data-urlencode "client_secret=${AUTH_D_VALIDATOR_1_VALIDATOR_CLIENT_SECRET}" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "scope=openid" | tr -d '\n' | grep -o -E '"access_token"[[:space:]]*:[[:space:]]*"[^"]+' | grep -o -E '[^"]+$')
export D_VALIDATOR_1_VALIDATOR_USER_TOKEN
