// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net.Http.Json;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Settings that drive the <c>client_credentials</c> grant: the token endpoint,
/// the audience to request, and the client id/secret pair. <paramref name="Scope"/>
/// is optional; if omitted, no <c>scope</c> form field is sent.
/// </summary>
public sealed record OAuth2TokenProviderOptions(
    Uri TokenEndpoint,
    string ClientId,
    string ClientSecret,
    string Audience,
    string? Scope = "openid")
{
    /// <summary>
    /// Safety margin used to consider an access token expired before its nominal
    /// expiry. Defaults to 30 seconds so callers don't race a clock-skewed
    /// validator into a 401.
    /// </summary>
    public TimeSpan ExpiryLeeway { get; init; } = TimeSpan.FromSeconds(30);
}

/// <summary>
/// Caches an OAuth2 access token in memory and refreshes it on expiry. A single
/// instance is safe to share across threads; concurrent callers will await one
/// in-flight refresh rather than fan out duplicate token requests.
/// </summary>
public sealed class OAuth2TokenProvider : IDisposable
{
    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProviderOptions _options;
    private readonly ILogger<OAuth2TokenProvider> _logger;
    private readonly TimeProvider _timeProvider;
    private readonly SemaphoreSlim _refreshLock = new(1, 1);

    private CachedToken? _cached;

    /// <summary>
    /// Creates a token provider for a single <c>client_credentials</c> client.
    /// </summary>
    /// <param name="httpClient">Client used to POST to the token endpoint.</param>
    /// <param name="options">Token endpoint, audience, scope, and client credentials.</param>
    /// <param name="logger">Optional logger; defaults to a no-op logger.</param>
    /// <param name="timeProvider">
    /// Clock used for cache-expiry decisions; defaults to
    /// <see cref="TimeProvider.System"/>. Override it in tests to drive expiry
    /// deterministically.
    /// </param>
    public OAuth2TokenProvider(
        HttpClient httpClient,
        OAuth2TokenProviderOptions options,
        ILogger<OAuth2TokenProvider>? logger = null,
        TimeProvider? timeProvider = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _options = options ?? throw new ArgumentNullException(nameof(options));
        _logger = logger ?? NullLogger<OAuth2TokenProvider>.Instance;
        _timeProvider = timeProvider ?? TimeProvider.System;
    }

    /// <summary>
    /// Returns a valid access token, refreshing if the cached token is missing
    /// or within the configured leeway of its expiry. Thread-safe.
    /// </summary>
    public async ValueTask<string> GetAccessTokenAsync(CancellationToken cancellationToken = default)
    {
        var cached = _cached;
        var now = _timeProvider.GetUtcNow();
        if (cached is not null && cached.IsValidAt(now, _options.ExpiryLeeway))
        {
            return cached.AccessToken;
        }

        await _refreshLock.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            cached = _cached;
            now = _timeProvider.GetUtcNow();
            if (cached is not null && cached.IsValidAt(now, _options.ExpiryLeeway))
            {
                return cached.AccessToken;
            }

            var fresh = await FetchAsync(cancellationToken).ConfigureAwait(false);
            _cached = fresh;
            return fresh.AccessToken;
        }
        finally
        {
            _refreshLock.Release();
        }
    }

    private async Task<CachedToken> FetchAsync(CancellationToken cancellationToken)
    {
        var form = new List<KeyValuePair<string, string>>
        {
            new("grant_type", "client_credentials"),
            new("client_id", _options.ClientId),
            new("client_secret", _options.ClientSecret),
            new("audience", _options.Audience),
        };
        if (!string.IsNullOrEmpty(_options.Scope))
        {
            form.Add(new KeyValuePair<string, string>("scope", _options.Scope));
        }

        using var request = new HttpRequestMessage(HttpMethod.Post, _options.TokenEndpoint)
        {
            Content = new FormUrlEncodedContent(form),
        };

        _logger.LogDebug(
            "Requesting OAuth2 token from {TokenEndpoint} for client {ClientId} audience {Audience}",
            _options.TokenEndpoint,
            _options.ClientId,
            _options.Audience);

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new OAuth2TokenException(
                $"Token endpoint {_options.TokenEndpoint} returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        var payload = await response.Content
            .ReadFromJsonAsync<TokenResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false)
            ?? throw new OAuth2TokenException(
                $"Token endpoint {_options.TokenEndpoint} returned an empty body.",
                response.StatusCode,
                string.Empty);

        if (string.IsNullOrEmpty(payload.AccessToken))
        {
            throw new OAuth2TokenException(
                $"Token endpoint {_options.TokenEndpoint} returned a response with no access_token.",
                response.StatusCode,
                string.Empty);
        }

        var lifetime = payload.ExpiresIn > 0 ? TimeSpan.FromSeconds(payload.ExpiresIn) : TimeSpan.FromMinutes(5);
        var expiresAt = _timeProvider.GetUtcNow() + lifetime;
        return new CachedToken(payload.AccessToken, expiresAt);
    }

    /// <summary>Releases the internal refresh lock.</summary>
    public void Dispose()
    {
        _refreshLock.Dispose();
    }

    private sealed record CachedToken(string AccessToken, DateTimeOffset ExpiresAtUtc)
    {
        public bool IsValidAt(DateTimeOffset now, TimeSpan leeway) => now + leeway < ExpiresAtUtc;
    }

    private sealed record TokenResponse(
        [property: JsonPropertyName("access_token")] string AccessToken,
        [property: JsonPropertyName("token_type")] string? TokenType,
        [property: JsonPropertyName("expires_in")] int ExpiresIn,
        [property: JsonPropertyName("scope")] string? Scope);
}

/// <summary>
/// Raised when the OAuth2 token endpoint returns a non-success status, an empty
/// body, or a body with no <c>access_token</c>.
/// </summary>
public sealed class OAuth2TokenException : Exception
{
    /// <summary>
    /// Creates the exception with the originating <paramref name="statusCode"/>
    /// and raw <paramref name="responseBody"/> for diagnostics.
    /// </summary>
    /// <param name="message">Human-readable description of the failure.</param>
    /// <param name="statusCode">HTTP status the token endpoint returned.</param>
    /// <param name="responseBody">Raw response body, or empty when none was read.</param>
    public OAuth2TokenException(string message, System.Net.HttpStatusCode statusCode, string responseBody)
        : base(message)
    {
        StatusCode = statusCode;
        ResponseBody = responseBody;
    }

    /// <summary>HTTP status the token endpoint returned.</summary>
    public System.Net.HttpStatusCode StatusCode { get; }

    /// <summary>Raw response body, or empty when none was read.</summary>
    public string ResponseBody { get; }
}
