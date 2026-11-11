// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Thin read-only wrapper over the Canton JSON Ledger API v2. It serves
/// <c>GET /v2/parties/participant-id</c> (<see cref="GetParticipantIdAsync"/>),
/// <c>GET /v2/state/connected-synchronizers</c> (<see cref="GetConnectedSynchronizersAsync"/>
/// and the app and global synchronizer id lookups built on it), and
/// <c>POST /v2/package-vetting/list</c> (<see cref="IsPackageVettedAsync"/>).
/// DAR upload, party allocation and user binding live in <see cref="DarUploader"/>,
/// <see cref="PartyAllocator"/> and <see cref="UserBuilder"/>.
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
            var body = await BoundedBody.ReadAsync(response.Content, cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"GET v2/parties/participant-id returned {(int)response.StatusCode} {response.ReasonPhrase}: {BoundedBody.ForMessage(body)}",
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

    /// <summary>The stable alias of the global synchronizer.</summary>
    public const string GlobalSynchronizerAlias = "global";

    /// <summary>
    /// Lists the synchronizers the participant is connected to, via
    /// <c>GET /v2/state/connected-synchronizers</c>. Pass <paramref name="party"/>
    /// to scope the result to that party; omit it (or pass <c>null</c>/empty) for
    /// the participant-wide list.
    /// </summary>
    public async Task<IReadOnlyList<ConnectedSynchronizer>> GetConnectedSynchronizersAsync(
        string? party = null,
        CancellationToken cancellationToken = default)
    {
        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        var requestUri = string.IsNullOrWhiteSpace(party)
            ? "v2/state/connected-synchronizers"
            : $"v2/state/connected-synchronizers?party={Uri.EscapeDataString(party)}";

        using var request = new HttpRequestMessage(HttpMethod.Get, requestUri);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("GET {Uri}", new Uri(_httpClient.BaseAddress!, request.RequestUri!));

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await BoundedBody.ReadAsync(response.Content, cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"GET {requestUri} returned {(int)response.StatusCode} {response.ReasonPhrase}: {BoundedBody.ForMessage(body)}",
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
    /// <see cref="AppSynchronizerAlias"/>, scoped to <paramref name="party"/>
    /// or participant-wide when it is <c>null</c> or empty. Throws if it is absent (multi-sync off).
    /// </summary>
    public async Task<string> GetAppSynchronizerIdAsync(
        string? party = null,
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

    /// <summary>
    /// Returns the id of the connected synchronizer whose alias is
    /// <see cref="GlobalSynchronizerAlias"/>. Every LocalNet profile connects to
    /// this synchronizer, single-sync or multi-sync, so it is the default
    /// target for party allocation (see <see cref="PartyAllocator"/>). Throws
    /// if no such synchronizer is connected.
    /// </summary>
    public async Task<string> GetGlobalSynchronizerIdAsync(
        string? party = null,
        CancellationToken cancellationToken = default)
    {
        var synchronizers = await GetConnectedSynchronizersAsync(party, cancellationToken).ConfigureAwait(false);
        var global = synchronizers.FirstOrDefault(s => string.Equals(s.Alias, GlobalSynchronizerAlias, StringComparison.Ordinal));
        if (global is null)
        {
            var aliases = string.Join(", ", synchronizers.Select(s => s.Alias));
            throw new JsonLedgerApiException(
                $"No connected synchronizer with alias '{GlobalSynchronizerAlias}'. Connected: [{aliases}].",
                System.Net.HttpStatusCode.NotFound,
                string.Empty);
        }

        return global.Id;
    }

    /// <summary>
    /// Checks whether this participant has vetted <paramref name="packageId"/>,
    /// via <c>POST /v2/package-vetting/list</c> filtered to the participant
    /// (<see cref="GetParticipantIdAsync"/>) and, when given, to
    /// <paramref name="synchronizerId"/>. Follows <c>nextPageToken</c> until a
    /// match or the last page. Used by <see cref="DarUploader"/> to tell an
    /// identical re-upload from a different package that shares the name and
    /// version of a vetted one.
    /// </summary>
    /// <param name="packageId">Package id to look for.</param>
    /// <param name="synchronizerId">Synchronizer whose topology state to read, or <c>null</c> for any.</param>
    /// <param name="cancellationToken">Cancels the requests.</param>
    /// <exception cref="JsonLedgerApiException">The participant returned a non-success status.</exception>
    internal async Task<bool> IsPackageVettedAsync(
        string packageId,
        string? synchronizerId,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(packageId))
        {
            throw new ArgumentException("packageId must be non-empty.", nameof(packageId));
        }

        var participantId = await GetParticipantIdAsync(cancellationToken).ConfigureAwait(false);
        var pageToken = string.Empty;
        do
        {
            var page = await PostVettingListAsync(packageId, participantId, synchronizerId, pageToken, cancellationToken).ConfigureAwait(false);
            var vetted = page.VettedPackages?.Any(entry =>
                string.Equals(entry.ParticipantId, participantId, StringComparison.Ordinal)
                && entry.Packages?.Any(p => string.Equals(p.PackageId, packageId, StringComparison.Ordinal)) == true) == true;
            if (vetted)
            {
                return true;
            }
            pageToken = page.NextPageToken ?? string.Empty;
        }
        while (pageToken.Length > 0);

        return false;
    }

    private async Task<VettingListResponse> PostVettingListAsync(
        string packageId,
        string participantId,
        string? synchronizerId,
        string pageToken,
        CancellationToken cancellationToken)
    {
        const string requestUri = "v2/package-vetting/list";
        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);

        using var request = new HttpRequestMessage(HttpMethod.Post, requestUri);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Content = JsonContent.Create(new VettingListRequest(
            new PackageMetadataFilter(new[] { packageId }),
            new TopologyStateFilter(
                new[] { participantId },
                synchronizerId is null ? null : new[] { synchronizerId }),
            pageToken.Length == 0 ? null : pageToken));

        _logger.LogDebug("POST {Uri}", new Uri(_httpClient.BaseAddress!, requestUri));

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var errorBody = await BoundedBody.ReadAsync(response.Content, cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"POST {requestUri} returned {(int)response.StatusCode} {response.ReasonPhrase}: {BoundedBody.ForMessage(errorBody)}",
                response.StatusCode,
                errorBody);
        }

        var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);

        return System.Text.Json.JsonSerializer.Deserialize<VettingListResponse>(body)
            ?? throw new JsonLedgerApiException($"POST {requestUri} returned an empty body.", response.StatusCode, body);
    }

    private sealed record ParticipantIdResponse(
        [property: JsonPropertyName("participantId")] string ParticipantId);

    private sealed record ConnectedSynchronizersResponse(
        [property: JsonPropertyName("connectedSynchronizers")] IReadOnlyList<ConnectedSynchronizerDto> ConnectedSynchronizers);

    private sealed record VettingListRequest(
        [property: JsonPropertyName("packageMetadataFilter")] PackageMetadataFilter PackageMetadataFilter,
        [property: JsonPropertyName("topologyStateFilter")] TopologyStateFilter TopologyStateFilter,
        [property: JsonPropertyName("pageToken"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] string? PageToken);

    private sealed record PackageMetadataFilter(
        [property: JsonPropertyName("packageIds")] IReadOnlyList<string> PackageIds);

    private sealed record TopologyStateFilter(
        [property: JsonPropertyName("participantIds")] IReadOnlyList<string> ParticipantIds,
        [property: JsonPropertyName("synchronizerIds"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] IReadOnlyList<string>? SynchronizerIds);

    private sealed record VettingListResponse(
        [property: JsonPropertyName("vettedPackages")] IReadOnlyList<VettedPackagesDto>? VettedPackages,
        [property: JsonPropertyName("nextPageToken")] string? NextPageToken);

    private sealed record VettedPackagesDto(
        [property: JsonPropertyName("participantId")] string? ParticipantId,
        [property: JsonPropertyName("packages")] IReadOnlyList<VettedPackageDto>? Packages);

    private sealed record VettedPackageDto(
        [property: JsonPropertyName("packageId")] string? PackageId);

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
