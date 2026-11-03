#!/bin/bash
# Copyright (c) 2026 Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -eou pipefail

check() {
  echo "Checking $1"
  # -Y off: busybox wget ignores no_proxy, so a proxy injected via the environment would break the probe
  wget -Y off -q --tries=1 --timeout=5 -O /dev/null "http://127.0.0.1:$1/health/ready"
}

if [ "$B_VALIDATOR_1_PROFILE" = "on" ]; then
  check "12${CANTON_HTTP_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$A_VALIDATOR_1_PROFILE" = "on" ]; then
  check "11${CANTON_HTTP_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$C_VALIDATOR_1_PROFILE" = "on" ]; then
  check "13${CANTON_HTTP_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$SV_VALIDATOR_1_PROFILE" = "on" ]; then
  check "10${CANTON_HTTP_HEALTHCHECK_PORT_SUFFIX}"
fi
if [ "$D_VALIDATOR_1_PROFILE" = "on" ]; then
  check "14${CANTON_HTTP_HEALTHCHECK_PORT_SUFFIX}"
fi
