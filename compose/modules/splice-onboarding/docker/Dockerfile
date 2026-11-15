# Copyright (c) 2026, Digital Asset (Switzerland) GmbH and/or its affiliates. All rights reserved.
# SPDX-License-Identifier: 0BSD

FROM sgaunet/jwt-cli:1.2.2@sha256:f49f1b56f810b7c61c0085dc5457a1fb0bfb77b98e0954bb6d6db53617ef1452 AS jwtcli

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
COPY --from=jwtcli /jwt-cli /usr/bin/jwt-cli
RUN apk add --no-cache jq curl bash
ENTRYPOINT ["/bin/bash"]
