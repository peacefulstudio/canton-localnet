#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -eou pipefail

CONSOLE_OP=""
CONSOLE_SCRIPT=""
if [[ $MULTI_SYNC == true ]]; then
  echo "Running in multi synchronizer mode"
  CONSOLE_OP="run"
  CONSOLE_SCRIPT="/app/app-synchronizer.sc"
fi

generate_jwt() {
  local sub="$1"
  local aud="$2"
  jwt-cli encode hs256 --s unsafe --p '{"sub": "'"$sub"'", "aud": "'"$aud"'"}'
}

A_VALIDATOR_1_VALIDATOR_USER_TOKEN=$(generate_jwt "$AUTH_A_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_A_VALIDATOR_1_AUDIENCE")
export A_VALIDATOR_1_VALIDATOR_USER_TOKEN
B_VALIDATOR_1_VALIDATOR_USER_TOKEN=$(generate_jwt "$AUTH_B_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_B_VALIDATOR_1_AUDIENCE")
export B_VALIDATOR_1_VALIDATOR_USER_TOKEN
SV_VALIDATOR_USER_TOKEN=$(generate_jwt "$AUTH_SV_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_SV_VALIDATOR_1_AUDIENCE")
export SV_VALIDATOR_USER_TOKEN
D_VALIDATOR_1_VALIDATOR_USER_TOKEN=$(generate_jwt "$AUTH_D_VALIDATOR_1_VALIDATOR_USER_NAME" "$AUTH_D_VALIDATOR_1_AUDIENCE")
export D_VALIDATOR_1_VALIDATOR_USER_TOKEN

# source all scripts from /app/pre-startup/on so that env variables exported by them are available in the current shell
for script in /app/pre-startup/on/*.sh; do
# shellcheck disable=SC1090
  [ -f "$script" ] && source "$script"
done
/app/bin/canton ${CONSOLE_OP} --no-tty -c /app/app.conf ${CONSOLE_SCRIPT}
