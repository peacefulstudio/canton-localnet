#!/bin/bash
# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

set -eo pipefail

source /app/utils.sh

if [ "$AUTH_MODE" = "oauth2" ]; then
  export A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN=$(get_admin_token $AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_SECRET $AUTH_A_VALIDATOR_1_VALIDATOR_CLIENT_ID $AUTH_A_VALIDATOR_1_TOKEN_URL)
  export A_VALIDATOR_1_PARTY=$(get_user_party "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_VALIDATOR_USER_ID "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}")

  if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/a-validator-1-init-user-cleanup ]; then
    # To update username in metadata
    update_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_WALLET_ADMIN_USER_ID $AUTH_A_VALIDATOR_1_WALLET_ADMIN_USER_NAME $A_VALIDATOR_1_PARTY "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    update_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_VALIDATOR_USER_ID $AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME $A_VALIDATOR_1_PARTY "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    delete_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" participant_admin "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    touch /tmp/a-validator-1-init-user-cleanup

  fi

else
  export A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN=$(generate_jwt "$AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_A_VALIDATOR_1_AUDIENCE")
  export A_VALIDATOR_1_PARTY=$(get_user_party "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}")

  if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/a-validator-1-init-user-cleanup ]; then
    delete_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" participant_admin "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    touch /tmp/a-validator-1-init-user-cleanup

  fi
fi
