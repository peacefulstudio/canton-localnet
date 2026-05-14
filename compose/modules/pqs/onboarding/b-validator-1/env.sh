#!/bin/bash
# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

set -eo pipefail

source /app/utils.sh

if [ "$AUTH_MODE" == "oauth2" ]; then
  # create user for pqs for b-validator-1
  create_user "$B_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_B_VALIDATOR_1_PQS_USER_ID $AUTH_B_VALIDATOR_1_PQS_USER_NAME "" "canton:12${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$B_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_B_VALIDATOR_1_PQS_USER_ID $B_VALIDATOR_1_PARTY "ReadAs" "canton:12${PARTICIPANT_JSON_API_PORT_SUFFIX}"

else
  create_user "$B_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_B_VALIDATOR_1_PQS_USER_NAME $AUTH_B_VALIDATOR_1_PQS_USER_NAME "" "canton:12${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$B_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_B_VALIDATOR_1_PQS_USER_NAME $B_VALIDATOR_1_PARTY "ReadAs" "canton:12${PARTICIPANT_JSON_API_PORT_SUFFIX}"

  # we need share token
  B_VALIDATOR_1_PQS_USER_TOKEN=$(generate_jwt "$AUTH_B_VALIDATOR_1_PQS_USER_NAME" "$AUTH_B_VALIDATOR_1_AUDIENCE")
  share_file "b-validator-1-pqs.conf" <<EOF
  pipeline.oauth.accessToken="${B_VALIDATOR_1_PQS_USER_TOKEN}"
EOF
fi
