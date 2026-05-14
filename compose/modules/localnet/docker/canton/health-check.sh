#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -eou pipefail

if [ "$B_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 12${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpcurl -plaintext "localhost:12${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}" grpc.health.v1.Health/Check
fi
if [ "$A_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 11${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpcurl -plaintext "localhost:11${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}" grpc.health.v1.Health/Check
fi
if [ "$SV_VALIDATOR_1_PROFILE" = "on" ]; then
  echo "Checking 10${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}"
  grpcurl -plaintext "localhost:10${CANTON_GRPC_HEALTHCHECK_PORT_SUFFIX}" grpc.health.v1.Health/Check
fi
