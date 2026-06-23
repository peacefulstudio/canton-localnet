#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -eou pipefail

if [ "$B_VALIDATOR_1_PROFILE" = "on" ]; then
  wget --no-verbose --tries=1 --spider "http://localhost:12${VALIDATOR_ADMIN_API_PORT_SUFFIX}/api/validator/readyz"
fi
if [ "$A_VALIDATOR_1_PROFILE" = "on" ]; then
  wget --no-verbose --tries=1 --spider "http://localhost:11${VALIDATOR_ADMIN_API_PORT_SUFFIX}/api/validator/readyz"
fi
if [ "$SV_VALIDATOR_1_PROFILE" = "on" ]; then
  wget --no-verbose --tries=1 --spider "http://localhost:10${VALIDATOR_ADMIN_API_PORT_SUFFIX}/api/validator/readyz"
  wget --no-verbose --tries=1 --spider http://localhost:5012/api/scan/readyz
  wget --no-verbose --tries=1 --spider http://localhost:5014/api/sv/readyz
fi
if [ "$D_VALIDATOR_1_PROFILE" = "on" ]; then
  wget --no-verbose --tries=1 --spider "http://localhost:14${VALIDATOR_ADMIN_API_PORT_SUFFIX}/api/validator/readyz"
fi
