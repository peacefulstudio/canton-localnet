#!/bin/bash
# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

set -eo pipefail

source /app/utils.sh

if [ "$AUTH_MODE" == "oauth2" ]; then
  # create user for pqs for a-validator-1
  create_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_PQS_USER_ID $AUTH_A_VALIDATOR_1_PQS_USER_NAME "" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_PQS_USER_ID $A_VALIDATOR_1_PARTY "ReadAs" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"

else
  create_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_PQS_USER_NAME $AUTH_A_VALIDATOR_1_PQS_USER_NAME "" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
  grant_rights "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_PQS_USER_NAME $A_VALIDATOR_1_PARTY "ReadAs" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"

  # we need share token
  A_VALIDATOR_1_PQS_USER_TOKEN=$(generate_jwt "$AUTH_A_VALIDATOR_1_PQS_USER_NAME" "$AUTH_A_VALIDATOR_1_AUDIENCE")
  share_file "a-validator-1-pqs.conf" <<EOF
  pipeline.oauth.accessToken="${A_VALIDATOR_1_PQS_USER_TOKEN}"
EOF
fi

