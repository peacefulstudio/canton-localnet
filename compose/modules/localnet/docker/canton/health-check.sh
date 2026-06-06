#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -eou pipefail

if [ "$B_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 12${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpc-health-probe -addr="localhost:12${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$A_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 11${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpc-health-probe -addr="localhost:11${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$SV_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 10${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpc-health-probe -addr="localhost:10${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$D_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 14${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpc-health-probe -addr="localhost:14${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
fi
