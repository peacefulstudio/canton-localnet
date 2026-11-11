# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD
ARG BASE_IMAGE

FROM sgaunet/jwt-cli:1.2.2@sha256:f49f1b56f810b7c61c0085dc5457a1fb0bfb77b98e0954bb6d6db53617ef1452 AS jwtcli

FROM $BASE_IMAGE
COPY --from=jwtcli /jwt-cli /usr/bin/jwt-cli

WORKDIR /app
ENTRYPOINT ["/usr/bin/tini", "--", "/app/entrypoint.sh"]
