// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

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
/// </summary>
public sealed class PartyAllocator
{
    private const string AllocatePath = "v2/parties";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<PartyAllocator> _logger;
    private readonly string _instanceSuffix;

    public PartyAllocator(
        HttpClient httpClient,
        OAuth2TokenProvider tokenProvider,
        ILogger<PartyAllocator>? logger = null,
        string? instanceSuffix = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _tokenProvider = tokenProvider ?? throw new ArgumentNullException(nameof(tokenProvider));
        _logger = logger ?? NullLogger<PartyAllocator>.Instance;
        _instanceSuffix = string.IsNullOrWhiteSpace(instanceSuffix) ? GenerateInstanceSuffix() : instanceSuffix;

        if (_httpClient.BaseAddress is null)
        {
            throw new ArgumentException(
                "HttpClient must have a BaseAddress set to the JSON Ledger API root (e.g. http://localhost:3975/).",
                nameof(httpClient));
        }
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
    /// Allocates a party with hint <c>&lt;consumerPrefix&gt;-&lt;instanceSuffix&gt;</c>.
    /// Returns the fully-qualified party id (<c>partyIdHint::&lt;namespace&gt;</c>).
    /// </summary>
    public async Task<AllocatedParty> AllocateAsync(
        string consumerPrefix,
        string? displayName = null,
        CancellationToken cancellationToken = default)
    {
        var hint = ComposeHint(consumerPrefix);
        return await AllocateWithHintAsync(hint, displayName ?? hint, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Lower-level overload that POSTs the supplied hint verbatim. Prefer
    /// <see cref="AllocateAsync(string, string?, CancellationToken)"/> in tests
    /// so the random suffix is applied.
    /// </summary>
    public async Task<AllocatedParty> AllocateWithHintAsync(
        string partyIdHint,
        string displayName,
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

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);

        using var request = new HttpRequestMessage(HttpMethod.Post, AllocatePath)
        {
            Content = JsonContent.Create(new AllocatePartyRequest(partyIdHint, displayName, string.Empty)),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("POST {Uri} (partyIdHint: {Hint})",
            new Uri(_httpClient.BaseAddress!, AllocatePath), partyIdHint);

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"POST {AllocatePath} (hint={partyIdHint}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

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

        _logger.LogInformation("Allocated party {PartyId} (hint={Hint})", payload.PartyDetails.Party, partyIdHint);
        return new AllocatedParty(payload.PartyDetails.Party, partyIdHint, payload.PartyDetails.IsLocal);
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
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId);

    private sealed record AllocatePartyResponse(
        [property: JsonPropertyName("partyDetails")] AllocatedPartyDetails? PartyDetails);

    private sealed record AllocatedPartyDetails(
        [property: JsonPropertyName("party")] string Party,
        [property: JsonPropertyName("isLocal")] bool IsLocal);
}

/// <summary>
/// Outcome of a party allocation: the fully-qualified party id plus the
/// hint that produced it.
/// </summary>
public sealed record AllocatedParty(string PartyId, string PartyIdHint, bool IsLocal);
