// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Text;
using System.Text.Json;

namespace Peaceful.Canton.Localnet.Testing.Tests;

internal sealed class ExpiringTokens
{
    private static readonly TimeSpan Lifetime = TimeSpan.FromSeconds(60);

    private int _issued;

    public ExpiringTokens()
    {
        Clock = new FakeTimeProvider(new DateTimeOffset(2026, 5, 10, 0, 0, 0, TimeSpan.Zero));
        Provider = new OAuth2TokenProvider(
            new HttpClient(new RecordingHandler(IssueToken)),
            new OAuth2TokenProviderOptions(
                TokenEndpoint: new Uri("https://keycloak.test/token"),
                ClientId: "id",
                ClientSecret: "sec",
                Audience: "aud",
                Scope: "openid"),
            timeProvider: Clock);
    }

    public FakeTimeProvider Clock { get; }

    public OAuth2TokenProvider Provider { get; }

    public Task ExpireCurrentTokenAsync(TimeSpan delay, CancellationToken cancellationToken)
    {
        Clock.Advance(Lifetime + Lifetime);
        return Task.CompletedTask;
    }

    private Task<HttpResponseMessage> IssueToken(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        var token = $"tok-{Interlocked.Increment(ref _issued)}";
        return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(
                JsonSerializer.Serialize(new { access_token = token, token_type = "Bearer", expires_in = (int)Lifetime.TotalSeconds }),
                Encoding.UTF8,
                "application/json"),
        });
    }
}
