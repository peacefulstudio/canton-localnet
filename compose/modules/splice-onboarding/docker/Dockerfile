# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

FROM sgaunet/jwt-cli:1.2.2@sha256:f49f1b56f810b7c61c0085dc5457a1fb0bfb77b98e0954bb6d6db53617ef1452 AS jwtcli

FROM alpine:3.24@sha256:3f4d6b32e26703c6082839234f43769d5046b3ac7c910033911d1e71d8cd630a
COPY --from=jwtcli /jwt-cli /usr/bin/jwt-cli
RUN apk add --no-cache jq curl bash
ENTRYPOINT ["/bin/bash"]
