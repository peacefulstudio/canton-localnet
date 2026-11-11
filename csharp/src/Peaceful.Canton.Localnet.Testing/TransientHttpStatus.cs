// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;

namespace Peaceful.Canton.Localnet.Testing;

internal static class TransientHttpStatus
{
    private const string RequestAlreadyInFlightMarker = "REQUEST_ALREADY_IN_FLIGHT";

    internal static bool IsTransient(HttpStatusCode status, string body) =>
        status is HttpStatusCode.ServiceUnavailable or HttpStatusCode.GatewayTimeout
        || (status == HttpStatusCode.Conflict && body.Contains(RequestAlreadyInFlightMarker, StringComparison.Ordinal));
}
