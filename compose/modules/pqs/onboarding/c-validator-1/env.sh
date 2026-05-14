#!/bin/bash
# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

set -eo pipefail

source /app/utils.sh

if [ "$AUTH_MODE" == "oauth2" ]; then
  # create user for pqs for c-validator-1
  create_user "$C_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_C_VALIDATOR_1_PQS_USER_ID $AUTH_C_VALIDATOR_1_PQS_USER_NAME "" "canton:13${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$C_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_C_VALIDATOR_1_PQS_USER_ID $C_VALIDATOR_1_PARTY "ReadAs" "canton:13${PARTICIPANT_JSON_API_PORT_SUFFIX}"

else
  create_user "$C_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_C_VALIDATOR_1_PQS_USER_NAME $AUTH_C_VALIDATOR_1_PQS_USER_NAME "" "canton:13${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$C_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_C_VALIDATOR_1_PQS_USER_NAME $C_VALIDATOR_1_PARTY "ReadAs" "canton:13${PARTICIPANT_JSON_API_PORT_SUFFIX}"

  # we need share token
  C_VALIDATOR_1_PQS_USER_TOKEN=$(generate_jwt "$AUTH_C_VALIDATOR_1_PQS_USER_NAME" "$AUTH_C_VALIDATOR_1_AUDIENCE")
  share_file "c-validator-1-pqs.conf" <<EOF
  pipeline.oauth.accessToken="${C_VALIDATOR_1_PQS_USER_TOKEN}"
EOF
fi
