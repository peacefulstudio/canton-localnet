// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Thin wrapper over the Canton JSON Ledger API v2. v0 implements only
/// <c>GET /v2/parties/participant-id</c>; subsequent slices (#9 onwards) will
/// add DAR upload, party allocation, and user binding.
/// </summary>
public sealed class JsonLedgerAdminClient
{
    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<JsonLedgerAdminClient> _logger;

    /// <summary>
    /// Creates a client bound to a JSON Ledger API <see cref="HttpClient"/>.
    /// </summary>
    /// <param name="httpClient">
    /// Client whose <see cref="HttpClient.BaseAddress"/> is the JSON Ledger API
    /// root (e.g. <c>http://localhost:11975/</c>). Required; an unset base
    /// address throws.
    /// </param>
    /// <param name="tokenProvider">Supplies the bearer token for each request.</param>
    /// <param name="logger">Optional logger; defaults to a no-op logger.</param>
    /// <exception cref="ArgumentException">
    /// <paramref name="httpClient"/> has no <see cref="HttpClient.BaseAddress"/>.
    /// </exception>
    public JsonLedgerAdminClient(
        HttpClient httpClient,
        OAuth2TokenProvider tokenProvider,
        ILogger<JsonLedgerAdminClient>? logger = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _tokenProvider = tokenProvider ?? throw new ArgumentNullException(nameof(tokenProvider));
        _logger = logger ?? NullLogger<JsonLedgerAdminClient>.Instance;

        if (_httpClient.BaseAddress is null)
        {
            throw new ArgumentException(
                "HttpClient must have a BaseAddress set to the JSON Ledger API root (e.g. http://localhost:11975/).",
                nameof(httpClient));
        }
    }

    /// <summary>
    /// Calls <c>GET /v2/parties/participant-id</c> and returns the participant's
    /// identifier (e.g. <c>a-validator-1::1220...</c>). Throws
    /// <see cref="JsonLedgerApiException"/> if the server returns a non-success
    /// status or an empty body.
    /// </summary>
    public async Task<string> GetParticipantIdAsync(CancellationToken cancellationToken = default)
    {
        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);

        using var request = new HttpRequestMessage(HttpMethod.Get, "v2/parties/participant-id");
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("GET {Uri}", new Uri(_httpClient.BaseAddress!, request.RequestUri!));

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"GET v2/parties/participant-id returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        var payload = await response.Content
            .ReadFromJsonAsync<ParticipantIdResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false)
            ?? throw new JsonLedgerApiException(
                "GET v2/parties/participant-id returned an empty body.",
                response.StatusCode,
                string.Empty);

        if (string.IsNullOrWhiteSpace(payload.ParticipantId))
        {
            throw new JsonLedgerApiException(
                "GET v2/parties/participant-id returned a response with no participantId field.",
                response.StatusCode,
                string.Empty);
        }

        return payload.ParticipantId;
    }

    /// <summary>The stable alias of the app-provider synchronizer.</summary>
    public const string AppSynchronizerAlias = "app-synchronizer";

    /// <summary>
    /// Lists the synchronizers the participant is connected to for <paramref name="party"/>,
    /// via <c>GET /v2/state/connected-synchronizers</c>.
    /// </summary>
    public async Task<IReadOnlyList<ConnectedSynchronizer>> GetConnectedSynchronizersAsync(
        string party,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(party))
        {
            throw new ArgumentException("party must be a non-empty party id.", nameof(party));
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        var requestUri = $"v2/state/connected-synchronizers?party={Uri.EscapeDataString(party)}";

        using var request = new HttpRequestMessage(HttpMethod.Get, requestUri);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("GET {Uri}", new Uri(_httpClient.BaseAddress!, request.RequestUri!));

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"GET {requestUri} returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        var payload = await response.Content
            .ReadFromJsonAsync<ConnectedSynchronizersResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false)
            ?? throw new JsonLedgerApiException(
                $"GET {requestUri} returned an empty body.",
                response.StatusCode,
                string.Empty);

        if (payload.ConnectedSynchronizers is null)
        {
            throw new JsonLedgerApiException(
                $"GET {requestUri} returned a response with no connectedSynchronizers field.",
                response.StatusCode,
                string.Empty);
        }

        foreach (var s in payload.ConnectedSynchronizers)
        {
            if (string.IsNullOrWhiteSpace(s.SynchronizerId))
            {
                throw new JsonLedgerApiException(
                    $"GET {requestUri} returned a connected-synchronizers entry with an empty synchronizerId (alias='{s.SynchronizerAlias}').",
                    response.StatusCode,
                    string.Empty);
            }
        }

        return payload.ConnectedSynchronizers
            .Select(s => new ConnectedSynchronizer(s.SynchronizerAlias, s.SynchronizerId, s.Permission))
            .ToList();
    }

    /// <summary>
    /// Returns the id of the connected synchronizer whose alias is
    /// <see cref="AppSynchronizerAlias"/>. Throws if it is absent (multi-sync off).
    /// </summary>
    public async Task<string> GetAppSynchronizerIdAsync(
        string party,
        CancellationToken cancellationToken = default)
    {
        var synchronizers = await GetConnectedSynchronizersAsync(party, cancellationToken).ConfigureAwait(false);
        var app = synchronizers.FirstOrDefault(s => string.Equals(s.Alias, AppSynchronizerAlias, StringComparison.Ordinal));
        if (app is null)
        {
            var aliases = string.Join(", ", synchronizers.Select(s => s.Alias));
            throw new JsonLedgerApiException(
                $"No connected synchronizer with alias '{AppSynchronizerAlias}' (is the multi-sync profile enabled?). Connected: [{aliases}].",
                System.Net.HttpStatusCode.NotFound,
                string.Empty);
        }

        return app.Id;
    }

    private sealed record ParticipantIdResponse(
        [property: JsonPropertyName("participantId")] string ParticipantId);

    private sealed record ConnectedSynchronizersResponse(
        [property: JsonPropertyName("connectedSynchronizers")] IReadOnlyList<ConnectedSynchronizerDto> ConnectedSynchronizers);

    private sealed record ConnectedSynchronizerDto(
        [property: JsonPropertyName("synchronizerAlias")] string SynchronizerAlias,
        [property: JsonPropertyName("synchronizerId")] string SynchronizerId,
        [property: JsonPropertyName("permission")] string? Permission);
}

/// <summary>
/// Raised when the JSON Ledger API returns a non-success status, an empty
/// response body, or a response that fails the v0 contract (missing fields).
/// <para>
/// This is a base type — <see cref="UserRightsGrantedWithoutLeaseException"/>
/// derives from it. A <c>catch</c> on this type catches the derived ones; xUnit's
/// <c>Assert.Throws&lt;T&gt;</c> and <c>ThrowsAsync&lt;T&gt;</c> match the exact
/// type and do not, so assert with <c>ThrowsAny</c> where a derived exception is
/// possible.
/// </para>
/// </summary>
public class JsonLedgerApiException : Exception
{
    /// <summary>
    /// Creates the exception with the originating <paramref name="statusCode"/>
    /// and raw <paramref name="responseBody"/> for diagnostics.
    /// </summary>
    /// <param name="message">Human-readable description of the failure.</param>
    /// <param name="statusCode">HTTP status the participant returned.</param>
    /// <param name="responseBody">Raw response body, or empty when none was read.</param>
    public JsonLedgerApiException(string message, System.Net.HttpStatusCode statusCode, string responseBody)
        : base(message)
    {
        StatusCode = statusCode;
        ResponseBody = responseBody;
    }

    /// <summary>HTTP status the participant returned.</summary>
    public System.Net.HttpStatusCode StatusCode { get; }

    /// <summary>Raw response body, or empty when none was read.</summary>
    public string ResponseBody { get; }
}
