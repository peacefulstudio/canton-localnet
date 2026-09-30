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

  if [ ! -f /tmp/a-validator-1-init-ci-clients-migrated ]; then
    migrate_realm_clients \
      "http://nginx-keycloak:8082/realms/master/protocol/openid-connect/token" \
      "http://nginx-keycloak:8082" \
      "AValidator1" \
      "a-validator-1-ci-" \
      "/app/keycloak/AValidator1-realm.json" \
      "/app/keycloak/AValidator1-users-0.json"
    touch /tmp/a-validator-1-init-ci-clients-migrated
  fi

  if [ ! -f /tmp/a-validator-1-init-ci-users-onboarded ]; then
    for ci_user in \
      "$AUTH_A_VALIDATOR_1_CI_1_USER_ID:a-validator-1-ci-1" \
      "$AUTH_A_VALIDATOR_1_CI_2_USER_ID:a-validator-1-ci-2" \
      "$AUTH_A_VALIDATOR_1_CI_3_USER_ID:a-validator-1-ci-3" \
      "$AUTH_A_VALIDATOR_1_CI_4_USER_ID:a-validator-1-ci-4" \
      "$AUTH_A_VALIDATOR_1_CI_PROVIDER_USER_ID:a-validator-1-ci-provider" \
      "$AUTH_A_VALIDATOR_1_CI_APP_USER_ID:a-validator-1-ci-app"; do
      ci_user_id="${ci_user%%:*}"
      ci_user_name="${ci_user##*:}"
      create_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "$ci_user_id" "service-account-$ci_user_name" "$A_VALIDATOR_1_PARTY" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
      grant_rights "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "$ci_user_id" "$A_VALIDATOR_1_PARTY" "ParticipantAdmin ReadAs ActAs" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    done
    touch /tmp/a-validator-1-init-ci-users-onboarded
  fi

else
  export A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN=$(generate_jwt "$AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_A_VALIDATOR_1_AUDIENCE")
  export A_VALIDATOR_1_PARTY=$(get_user_party "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" $AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}")

  if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/a-validator-1-init-user-cleanup ]; then
    delete_user "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" participant_admin "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
    touch /tmp/a-validator-1-init-user-cleanup

  fi
fi
