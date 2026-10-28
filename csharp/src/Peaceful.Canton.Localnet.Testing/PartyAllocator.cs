// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Allocates parties on the Canton JSON Ledger API
/// (<c>POST /v2/parties</c>) using bearer tokens from
/// <see cref="OAuth2TokenProvider"/>. Each fixture instance gets a single
/// random suffix at construction time; party hints are composed as
/// <c>&lt;consumer-prefix&gt;-&lt;instance-suffix&gt;</c> so concurrent
/// test runs and reruns against a long-lived stack don't collide.
/// Unless a caller passes an explicit synchronizer id, the party is
/// allocated on the global synchronizer, resolved by alias — this works
/// unchanged on both single-sync and multi-sync stacks.
/// </summary>
public sealed class PartyAllocator
{
    private const string AllocatePath = "v2/parties";
    private const string PartyAlreadyExistsMarker = "Party already exists";
    private const string RequestAlreadyInFlightMarker = "REQUEST_ALREADY_IN_FLIGHT";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<PartyAllocator> _logger;
    private readonly string _instanceSuffix;
    private readonly JsonLedgerAdminClient _adminClient;
    private readonly DarUploaderRetryOptions _retryOptions;

    /// <summary>
    /// Creates an allocator bound to a JSON Ledger API <see cref="HttpClient"/>.
    /// </summary>
    /// <param name="httpClient">
    /// Client whose <see cref="HttpClient.BaseAddress"/> is the JSON Ledger API
    /// root (e.g. <c>http://localhost:11975/</c>). Required; an unset base
    /// address throws.
    /// </param>
    /// <param name="tokenProvider">Supplies the bearer token for each request.</param>
    /// <param name="logger">Optional logger; defaults to a no-op logger.</param>
    /// <param name="instanceSuffix">
    /// Fixed suffix appended to every party hint; defaults to a fresh random
    /// hex value so concurrent runs and reruns don't collide. Pass an explicit
    /// value when a deterministic hint is needed.
    /// </param>
    /// <param name="adminClient">
    /// Admin client used for synchronizer discovery; defaults to a private,
    /// unlogged client over <paramref name="httpClient"/>.
    /// <see cref="LocalnetFixture"/> passes the one it registers.
    /// </param>
    /// <param name="retryOptions">
    /// Retry policy for a transient <c>503</c> or <c>504</c>, a <c>409</c>
    /// <c>REQUEST_ALREADY_IN_FLIGHT</c> or a request timeout from
    /// <c>POST /v2/parties</c>; defaults to
    /// <see cref="DarUploaderRetryOptions.Default"/>, the policy
    /// <see cref="DarUploader"/> uses.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="httpClient"/> has no <see cref="HttpClient.BaseAddress"/>.
    /// </exception>
    public PartyAllocator(
        HttpClient httpClient,
        OAuth2TokenProvider tokenProvider,
        ILogger<PartyAllocator>? logger = null,
        string? instanceSuffix = null,
        JsonLedgerAdminClient? adminClient = null,
        DarUploaderRetryOptions? retryOptions = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _tokenProvider = tokenProvider ?? throw new ArgumentNullException(nameof(tokenProvider));
        _logger = logger ?? NullLogger<PartyAllocator>.Instance;
        _retryOptions = retryOptions ?? DarUploaderRetryOptions.Default;
        _instanceSuffix = string.IsNullOrWhiteSpace(instanceSuffix) ? GenerateInstanceSuffix() : instanceSuffix;

        if (_httpClient.BaseAddress is null)
        {
            throw new ArgumentException(
                "HttpClient must have a BaseAddress set to the JSON Ledger API root (e.g. http://localhost:11975/).",
                nameof(httpClient));
        }

        _adminClient = adminClient ?? new JsonLedgerAdminClient(_httpClient, _tokenProvider);
    }

    /// <summary>
    /// Stable per-instance suffix used in every <see cref="ComposeHint"/> call.
    /// Useful for assertions and log scoping.
    /// </summary>
    public string InstanceSuffix => _instanceSuffix;

    /// <summary>
    /// Returns the party-id hint that would be used for
    /// <paramref name="consumerPrefix"/> on this fixture instance.
    /// </summary>
    public string ComposeHint(string consumerPrefix)
    {
        if (string.IsNullOrWhiteSpace(consumerPrefix))
        {
            throw new ArgumentException("Consumer prefix must be non-empty.", nameof(consumerPrefix));
        }
        return $"{consumerPrefix}-{_instanceSuffix}";
    }

    /// <summary>
    /// Allocates a party with hint <c>&lt;consumerPrefix&gt;-&lt;instanceSuffix&gt;</c>
    /// on the global synchronizer, resolved by
    /// <see cref="JsonLedgerAdminClient.GlobalSynchronizerAlias"/>. Returns the
    /// fully-qualified party id (<c>partyIdHint::&lt;namespace&gt;</c>).
    /// </summary>
    /// <param name="consumerPrefix">Caller-chosen prefix for the party hint.</param>
    /// <param name="displayName">Optional display name; defaults to the hint.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public async Task<AllocatedParty> AllocateAsync(
        string consumerPrefix,
        string? displayName = null,
        CancellationToken cancellationToken = default)
    {
        var hint = ComposeHint(consumerPrefix);
        return await AllocateWithHintAsync(hint, displayName ?? hint, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Allocates a party with hint <c>&lt;consumerPrefix&gt;-&lt;instanceSuffix&gt;</c>
    /// on <paramref name="synchronizerId"/> instead of the global synchronizer
    /// default. Returns the fully-qualified party id (<c>partyIdHint::&lt;namespace&gt;</c>).
    /// </summary>
    /// <param name="consumerPrefix">Caller-chosen prefix for the party hint.</param>
    /// <param name="displayName">Display name; pass <see langword="null"/> to default to the hint.</param>
    /// <param name="synchronizerId">Synchronizer to allocate the party on.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public async Task<AllocatedParty> AllocateOnSynchronizerAsync(
        string consumerPrefix,
        string? displayName,
        string synchronizerId,
        CancellationToken cancellationToken = default)
    {
        var hint = ComposeHint(consumerPrefix);
        return await AllocateWithHintOnSynchronizerAsync(hint, displayName ?? hint, synchronizerId, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Lower-level overload that POSTs the supplied hint verbatim on the
    /// global synchronizer, resolved by
    /// <see cref="JsonLedgerAdminClient.GlobalSynchronizerAlias"/>. Prefer
    /// <see cref="AllocateAsync(string, string?, CancellationToken)"/> in
    /// tests so the random suffix is applied.
    /// </summary>
    /// <param name="partyIdHint">Party hint sent verbatim.</param>
    /// <param name="displayName">Display name.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public async Task<AllocatedParty> AllocateWithHintAsync(
        string partyIdHint,
        string displayName,
        CancellationToken cancellationToken = default)
        => await AllocateWithHintOnSynchronizerAsync(partyIdHint, displayName, synchronizerId: null, cancellationToken)
            .ConfigureAwait(false);

    /// <summary>
    /// Lower-level overload that POSTs the supplied hint verbatim on
    /// <paramref name="synchronizerId"/> instead of the global synchronizer
    /// default. Prefer
    /// <see cref="AllocateOnSynchronizerAsync(string, string?, string, CancellationToken)"/>
    /// in tests so the random suffix is applied.
    /// </summary>
    /// <param name="partyIdHint">Party hint sent verbatim.</param>
    /// <param name="displayName">Display name.</param>
    /// <param name="synchronizerId">Synchronizer to allocate the party on.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public async Task<AllocatedParty> AllocateWithHintOnSynchronizerAsync(
        string partyIdHint,
        string displayName,
        string? synchronizerId,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(partyIdHint))
        {
            throw new ArgumentException("Party hint must be non-empty.", nameof(partyIdHint));
        }
        if (string.IsNullOrWhiteSpace(displayName))
        {
            throw new ArgumentException("Display name must be non-empty.", nameof(displayName));
        }

        var resolvedSynchronizerId = string.IsNullOrWhiteSpace(synchronizerId)
            ? await _adminClient.GetGlobalSynchronizerIdAsync(cancellationToken: cancellationToken).ConfigureAwait(false)
            : synchronizerId;

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);

        var details = await PostWithRetryAsync(partyIdHint, displayName, resolvedSynchronizerId, token, cancellationToken).ConfigureAwait(false);

        _logger.LogInformation("Allocated party {PartyId} (hint={Hint})", details.Party, partyIdHint);
        return new AllocatedParty(details.Party, partyIdHint, details.IsLocal);
    }

    private async Task<AllocatedPartyDetails> PostWithRetryAsync(
        string partyIdHint,
        string displayName,
        string synchronizerId,
        string token,
        CancellationToken cancellationToken)
    {
        var earlierAttemptInconclusive = false;
        for (var attempt = 1; ; attempt++)
        {
            _logger.LogDebug("POST {Uri} (partyIdHint: {Hint})",
                new Uri(_httpClient.BaseAddress!, AllocatePath), partyIdHint);

            HttpResponseMessage? response = null;
            try
            {
                response = await SendAllocationAsync(partyIdHint, displayName, synchronizerId, token, cancellationToken).ConfigureAwait(false);
            }
            catch (TaskCanceledException timeout) when (!cancellationToken.IsCancellationRequested)
            {
                if (attempt >= _retryOptions.MaxAttempts)
                {
                    throw new TimeoutException(
                        $"POST {AllocatePath} (hint={partyIdHint}) timed out on all {_retryOptions.MaxAttempts} attempts.",
                        timeout);
                }
                await BackOffAsync(partyIdHint, "timed out", attempt, cancellationToken).ConfigureAwait(false);
                earlierAttemptInconclusive = true;
                continue;
            }

            using (response)
            {
                if (response.IsSuccessStatusCode)
                {
                    return await ReadAllocatedPartyAsync(response, cancellationToken).ConfigureAwait(false);
                }

                var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
                if (IsTransient(response.StatusCode, body))
                {
                    if (attempt >= _retryOptions.MaxAttempts)
                    {
                        throw new JsonLedgerApiException(
                            $"POST {AllocatePath} (hint={partyIdHint}) returned {(int)response.StatusCode} {response.ReasonPhrase} on all {_retryOptions.MaxAttempts} attempts: {body}",
                            response.StatusCode,
                            body);
                    }
                    await BackOffAsync(partyIdHint, $"returned {(int)response.StatusCode} {response.ReasonPhrase}", attempt, cancellationToken).ConfigureAwait(false);
                    earlierAttemptInconclusive = true;
                    continue;
                }

                if (earlierAttemptInconclusive && IsPartyAlreadyExists(response.StatusCode, body))
                {
                    var existing = await FindPartyByHintAsync(partyIdHint, token, cancellationToken).ConfigureAwait(false);
                    if (existing is not null)
                    {
                        _logger.LogInformation(
                            "Party {PartyId} (hint={Hint}) already exists after an earlier attempt did not complete — treating as success.",
                            existing.Party, partyIdHint);
                        return existing;
                    }
                }

                throw new JsonLedgerApiException(
                    $"POST {AllocatePath} (hint={partyIdHint}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                    response.StatusCode,
                    body);
            }
        }
    }

    private Task<HttpResponseMessage> SendAllocationAsync(
        string partyIdHint,
        string displayName,
        string synchronizerId,
        string token,
        CancellationToken cancellationToken)
    {
        var request = new HttpRequestMessage(HttpMethod.Post, AllocatePath)
        {
            Content = JsonContent.Create(new AllocatePartyRequest(partyIdHint, displayName, string.Empty, synchronizerId)),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
        return SendAndDisposeAsync(request, cancellationToken);
    }

    private async Task<HttpResponseMessage> SendAndDisposeAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        using (request)
        {
            return await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        }
    }

    private async Task BackOffAsync(string partyIdHint, string outcome, int attempt, CancellationToken cancellationToken)
    {
        var delay = _retryOptions.DelayForAttempt(attempt);
        _logger.LogWarning(
            "POST {Path} (hint={Hint}) {Outcome} (attempt {Attempt}/{MaxAttempts}); retrying in {Delay}.",
            AllocatePath, partyIdHint, outcome, attempt, _retryOptions.MaxAttempts, delay);
        await _retryOptions.Delay(delay, cancellationToken).ConfigureAwait(false);
    }

    private static bool IsTransient(HttpStatusCode status, string body) =>
        status is HttpStatusCode.ServiceUnavailable or HttpStatusCode.GatewayTimeout
        || (status == HttpStatusCode.Conflict && body.Contains(RequestAlreadyInFlightMarker, StringComparison.Ordinal));

    private static bool IsPartyAlreadyExists(HttpStatusCode status, string body) =>
        status == HttpStatusCode.BadRequest && body.Contains(PartyAlreadyExistsMarker, StringComparison.Ordinal);

    private static async Task<AllocatedPartyDetails> ReadAllocatedPartyAsync(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        var payload = await response.Content
            .ReadFromJsonAsync<AllocatePartyResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false)
            ?? throw new JsonLedgerApiException(
                $"POST {AllocatePath} returned an empty body.",
                response.StatusCode,
                string.Empty);

        if (payload.PartyDetails is null || string.IsNullOrWhiteSpace(payload.PartyDetails.Party))
        {
            throw new JsonLedgerApiException(
                $"POST {AllocatePath} returned a response with no partyDetails.party field.",
                response.StatusCode,
                string.Empty);
        }

        return payload.PartyDetails;
    }

    private async Task<AllocatedPartyDetails?> FindPartyByHintAsync(
        string partyIdHint,
        string token,
        CancellationToken cancellationToken)
    {
        var partyIdPrefix = $"{partyIdHint}::";
        var requestUri = $"{AllocatePath}?filter-party={Uri.EscapeDataString(partyIdPrefix)}";
        using var request = new HttpRequestMessage(HttpMethod.Get, requestUri);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

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
            .ReadFromJsonAsync<ListKnownPartiesResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false);
        return payload?.PartyDetails?.FirstOrDefault(
            details => details.Party.StartsWith(partyIdPrefix, StringComparison.Ordinal));
    }

    private static string GenerateInstanceSuffix()
    {
        var buffer = new byte[6];
        RandomNumberGenerator.Fill(buffer);
        return Convert.ToHexString(buffer).ToLowerInvariant();
    }

    private sealed record AllocatePartyRequest(
        [property: JsonPropertyName("partyIdHint")] string PartyIdHint,
        [property: JsonPropertyName("displayName")] string DisplayName,
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId,
        [property: JsonPropertyName("synchronizerId")] string SynchronizerId);

    private sealed record AllocatePartyResponse(
        [property: JsonPropertyName("partyDetails")] AllocatedPartyDetails? PartyDetails);

    private sealed record ListKnownPartiesResponse(
        [property: JsonPropertyName("partyDetails")] List<AllocatedPartyDetails>? PartyDetails);

    private sealed record AllocatedPartyDetails(
        [property: JsonPropertyName("party")] string Party,
        [property: JsonPropertyName("isLocal")] bool IsLocal);
}

/// <summary>
/// Outcome of a party allocation: the fully-qualified party id plus the
/// hint that produced it.
/// </summary>
public sealed record AllocatedParty(string PartyId, string PartyIdHint, bool IsLocal);
