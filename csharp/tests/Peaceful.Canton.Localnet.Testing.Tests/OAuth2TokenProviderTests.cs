// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class OAuth2TokenProviderTests
{
    private static OAuth2TokenProviderOptions DefaultOptions(string? scope = "openid") => new(
        TokenEndpoint: new Uri("https://keycloak.test/realms/AppProvider/protocol/openid-connect/token"),
        ClientId: "test-client",
        ClientSecret: "shhh",
        Audience: "https://canton.network.global",
        Scope: scope);

    private static HttpResponseMessage TokenResponse(string accessToken, int expiresIn = 300)
    {
        var payload = new
        {
            access_token = accessToken,
            token_type = "Bearer",
            expires_in = expiresIn,
            scope = "openid",
        };
        var json = JsonSerializer.Serialize(payload);
        return new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };
    }

    [Fact]
    public async Task GetAccessTokenAsync_sends_form_encoded_client_credentials_grant()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(TokenResponse("tok-1")));
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions());

        var token = await provider.GetAccessTokenAsync();

        Assert.Equal("tok-1", token);
        var recorded = Assert.Single(handler.Requests);
        Assert.Equal(HttpMethod.Post, recorded.Method);
        Assert.Equal(DefaultOptions().TokenEndpoint, recorded.Uri);
        var form = ParseForm(recorded.Body);
        Assert.Equal("client_credentials", form["grant_type"]);
        Assert.Equal("test-client", form["client_id"]);
        Assert.Equal("shhh", form["client_secret"]);
        Assert.Equal("https://canton.network.global", form["audience"]);
        Assert.Equal("openid", form["scope"]);
    }

    [Fact]
    public async Task GetAccessTokenAsync_omits_scope_when_options_scope_is_null()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(TokenResponse("tok-1")));
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions(scope: null));

        await provider.GetAccessTokenAsync();

        var recorded = Assert.Single(handler.Requests);
        var form = ParseForm(recorded.Body);
        Assert.False(form.ContainsKey("scope"));
    }

    [Fact]
    public async Task GetAccessTokenAsync_caches_token_until_expiry()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(TokenResponse($"tok-{calls}", expiresIn: 600));
        });
        using var http = new HttpClient(handler);
        var clock = new FakeTimeProvider(new DateTimeOffset(2026, 5, 10, 0, 0, 0, TimeSpan.Zero));
        using var provider = new OAuth2TokenProvider(http, DefaultOptions(), timeProvider: clock);

        var first = await provider.GetAccessTokenAsync();
        clock.Advance(TimeSpan.FromMinutes(5));
        var second = await provider.GetAccessTokenAsync();

        Assert.Equal("tok-1", first);
        Assert.Equal("tok-1", second);
        Assert.Equal(1, calls);
    }

    [Fact]
    public async Task GetAccessTokenAsync_refreshes_when_token_expired()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(TokenResponse($"tok-{calls}", expiresIn: 60));
        });
        using var http = new HttpClient(handler);
        var clock = new FakeTimeProvider(new DateTimeOffset(2026, 5, 10, 0, 0, 0, TimeSpan.Zero));
        using var provider = new OAuth2TokenProvider(http, DefaultOptions(), timeProvider: clock);

        var first = await provider.GetAccessTokenAsync();
        clock.Advance(TimeSpan.FromSeconds(120));
        var second = await provider.GetAccessTokenAsync();

        Assert.Equal("tok-1", first);
        Assert.Equal("tok-2", second);
        Assert.Equal(2, calls);
    }

    [Fact]
    public async Task GetAccessTokenAsync_refreshes_within_leeway_window()
    {
        var calls = 0;
        var handler = new RecordingHandler((_, _) =>
        {
            calls++;
            return Task.FromResult(TokenResponse($"tok-{calls}", expiresIn: 60));
        });
        using var http = new HttpClient(handler);
        var clock = new FakeTimeProvider(new DateTimeOffset(2026, 5, 10, 0, 0, 0, TimeSpan.Zero));
        var options = DefaultOptions() with { ExpiryLeeway = TimeSpan.FromSeconds(30) };
        using var provider = new OAuth2TokenProvider(http, options, timeProvider: clock);

        var first = await provider.GetAccessTokenAsync();
        clock.Advance(TimeSpan.FromSeconds(40));
        var second = await provider.GetAccessTokenAsync();

        Assert.Equal("tok-1", first);
        Assert.Equal("tok-2", second);
        Assert.Equal(2, calls);
    }

    [Fact]
    public async Task GetAccessTokenAsync_throws_OAuth2TokenException_on_non_success_status()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Unauthorized)
        {
            Content = new StringContent("""{"error":"invalid_client"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions());

        var exception = await Assert.ThrowsAsync<OAuth2TokenException>(() => provider.GetAccessTokenAsync().AsTask());
        Assert.Equal(HttpStatusCode.Unauthorized, exception.StatusCode);
        Assert.Contains("invalid_client", exception.ResponseBody);
    }

    [Fact]
    public async Task GetAccessTokenAsync_throws_when_body_has_no_access_token()
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("""{"token_type":"Bearer"}""", Encoding.UTF8, "application/json"),
        }));
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions());

        await Assert.ThrowsAsync<OAuth2TokenException>(() => provider.GetAccessTokenAsync().AsTask());
    }

    [Fact]
    public async Task GetAccessTokenAsync_serializes_concurrent_refreshes()
    {
        var calls = 0;
        var release = new TaskCompletionSource();
        var handler = new RecordingHandler(async (_, ct) =>
        {
            Interlocked.Increment(ref calls);
            await release.Task.WaitAsync(ct).ConfigureAwait(false);
            return TokenResponse($"tok-{calls}");
        });
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions());

        var t1 = provider.GetAccessTokenAsync().AsTask();
        var t2 = provider.GetAccessTokenAsync().AsTask();
        release.SetResult();
        var results = await Task.WhenAll(t1, t2);

        Assert.Equal(results[0], results[1]);
        Assert.Equal(1, calls);
    }

    [Theory]
    [InlineData("openid")]
    [InlineData("openid profile")]
    [InlineData("daml_ledger_api")]
    public async Task GetAccessTokenAsync_forwards_arbitrary_scope_strings(string scope)
    {
        var handler = new RecordingHandler((_, _) => Task.FromResult(TokenResponse("tok-1")));
        using var http = new HttpClient(handler);
        using var provider = new OAuth2TokenProvider(http, DefaultOptions(scope));

        await provider.GetAccessTokenAsync();

        var recorded = Assert.Single(handler.Requests);
        var form = ParseForm(recorded.Body);
        Assert.Equal(scope, form["scope"]);
    }

    private static Dictionary<string, string> ParseForm(string body)
    {
        return body
            .Split('&', StringSplitOptions.RemoveEmptyEntries)
            .Select(pair => pair.Split('=', 2))
            .ToDictionary(
                kv => DecodeFormValue(kv[0]),
                kv => kv.Length == 2 ? DecodeFormValue(kv[1]) : string.Empty,
                StringComparer.Ordinal);
    }

    private static string DecodeFormValue(string raw) => Uri.UnescapeDataString(raw.Replace('+', ' '));
}

internal sealed class FakeTimeProvider : TimeProvider
{
    private DateTimeOffset _now;

    public FakeTimeProvider(DateTimeOffset start)
    {
        _now = start;
    }

    public override DateTimeOffset GetUtcNow() => _now;

    public void Advance(TimeSpan by)
    {
        _now += by;
    }
}
