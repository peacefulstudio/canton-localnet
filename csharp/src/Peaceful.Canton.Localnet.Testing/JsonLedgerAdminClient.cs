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

    private sealed record ParticipantIdResponse(
        [property: JsonPropertyName("participantId")] string ParticipantId);
}

/// <summary>
/// Raised when the JSON Ledger API returns a non-success status, an empty
/// response body, or a response that fails the v0 contract (missing fields).
/// </summary>
public sealed class JsonLedgerApiException : Exception
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
