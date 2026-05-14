#!/bin/bash
# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

set -eo pipefail
trap 'touch /tmp/error' ERR
exec > /proc/1/fd/1 2>&1

if [ ! -f /tmp/all-done ]; then
  ONBOARDING_SCRIPTS_DIR="/app/scripts/on"
  ONBOARDING_TEMP_DIR="/tmp/onboarding-scripts/$(hostname)"

  if [ ! -d "$ONBOARDING_TEMP_DIR" ]; then
    mkdir -p "$ONBOARDING_TEMP_DIR"
  fi

  if [ -f /app/do-init ]; then
    echo "Initializing ..."
    export DO_INIT=true
  fi
  source /app/utils.sh

  if [ "$A_VALIDATOR_1_PROFILE" == "on" ]; then
    source /app/a-validator-1-auth.sh
    if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/a-validator-1-init-dars-uploaded ]; then
      upload_dars "$A_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "canton:11${PARTICIPANT_JSON_API_PORT_SUFFIX}"
      touch /tmp/a-validator-1-init-dars-uploaded
    fi
  fi

  if [ "$B_VALIDATOR_1_PROFILE" == "on" ]; then
    source /app/b-validator-1-auth.sh
    if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/b-validator-1-init-dars-uploaded ]; then
      upload_dars "$B_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "canton:12${PARTICIPANT_JSON_API_PORT_SUFFIX}"
      touch /tmp/b-validator-1-init-dars-uploaded
    fi
  fi

  if [ "$C_VALIDATOR_1_PROFILE" == "on" ]; then
    source /app/c-validator-1-auth.sh
    if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/c-validator-1-init-dars-uploaded ]; then
      upload_dars "$C_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "canton:13${PARTICIPANT_JSON_API_PORT_SUFFIX}"
      touch /tmp/c-validator-1-init-dars-uploaded
    fi
  fi

  if [ "$D_VALIDATOR_1_PROFILE" == "on" ]; then
    source /app/d-validator-1-auth.sh
    if [ "$DO_INIT" == "true" ] && [ ! -f /tmp/d-validator-1-init-dars-uploaded ]; then
      upload_dars "$D_VALIDATOR_1_PARTICIPANT_ADMIN_TOKEN" "canton:14${PARTICIPANT_JSON_API_PORT_SUFFIX}"
      touch /tmp/d-validator-1-init-dars-uploaded
    fi
  fi

  echo "Executing onboarding scripts..." >&2

  for script in $(ls "$ONBOARDING_SCRIPTS_DIR"); do
    script_name=$(basename "$script")
    done_file="$ONBOARDING_TEMP_DIR/${script_name}.done"

    if [ ! -f "$done_file" ]; then
      echo "executing $script_name" >&2
      chmod +x "$ONBOARDING_SCRIPTS_DIR/$script"
      "$ONBOARDING_SCRIPTS_DIR/$script"
      echo "$script_name done" >&2
      touch "$done_file"
    fi
  done
  touch /tmp/all-done
fi

