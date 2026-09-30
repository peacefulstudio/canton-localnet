// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;

namespace Peaceful.Canton.Localnet.Testing.Tests;

internal static class SynchronizerDiscoveryResponder
{
    public const string DefaultGlobalSynchronizerId = "global::122a";

    public static Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> WithConnectedSynchronizers(
        Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> inner,
        params (string Alias, string Id)[] synchronizers)
    {
        var effective = synchronizers.Length == 0
            ? new (string Alias, string Id)[] { ("global", DefaultGlobalSynchronizerId) }
            : synchronizers;

        return (request, cancellationToken) =>
        {
            if (request.Method == HttpMethod.Get
                && request.RequestUri is not null
                && request.RequestUri.AbsolutePath.TrimEnd('/').EndsWith("connected-synchronizers", StringComparison.Ordinal))
            {
                var entries = string.Join(",", effective.Select(s =>
                    $$"""{"synchronizerAlias":"{{s.Alias}}","synchronizerId":"{{s.Id}}"}"""));
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent($$"""{"connectedSynchronizers":[{{entries}}]}""", Encoding.UTF8, "application/json"),
                });
            }

            return inner(request, cancellationToken);
        };
    }
}
