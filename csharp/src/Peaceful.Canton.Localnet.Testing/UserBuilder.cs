// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Creates Canton ledger users (<c>POST /v2/users</c>) and grants
/// <c>CanActAs</c>/<c>CanReadAs</c> rights
/// (<c>POST /v2/users/{id}/rights</c>) using bearer tokens from
/// <see cref="OAuth2TokenProvider"/>.
/// </summary>
public sealed class UserBuilder
{
    private const string UsersPath = "v2/users";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<UserBuilder> _logger;

    public UserBuilder(
        HttpClient httpClient,
        OAuth2TokenProvider tokenProvider,
        ILogger<UserBuilder>? logger = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _tokenProvider = tokenProvider ?? throw new ArgumentNullException(nameof(tokenProvider));
        _logger = logger ?? NullLogger<UserBuilder>.Instance;

        if (_httpClient.BaseAddress is null)
        {
            throw new ArgumentException(
                "HttpClient must have a BaseAddress set to the JSON Ledger API root (e.g. http://localhost:11975/).",
                nameof(httpClient));
        }
    }

    /// <summary>
    /// Creates a ledger user with the supplied <paramref name="userId"/> and
    /// optionally grants the listed <paramref name="actAs"/> /
    /// <paramref name="readAs"/> rights in a follow-up
    /// <c>POST /v2/users/{id}/rights</c> request. Returns the created user id
    /// as reported by the participant.
    /// </summary>
    public async Task<string> CreateAsync(
        string userId,
        string? primaryParty = null,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(userId))
        {
            throw new ArgumentException("User id must be non-empty.", nameof(userId));
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        var created = await PostCreateUserAsync(token, userId, primaryParty, cancellationToken).ConfigureAwait(false);

        var actAsList = (actAs ?? Array.Empty<string>()).ToArray();
        var readAsList = (readAs ?? Array.Empty<string>()).ToArray();
        if (actAsList.Length > 0 || readAsList.Length > 0)
        {
            await PostGrantRightsAsync(token, created, actAsList, readAsList, cancellationToken).ConfigureAwait(false);
        }

        return created;
    }

    private async Task<string> PostCreateUserAsync(
        string token,
        string userId,
        string? primaryParty,
        CancellationToken cancellationToken)
    {
        using var request = new HttpRequestMessage(HttpMethod.Post, UsersPath)
        {
            Content = JsonContent.Create(new CreateUserRequest(
                User: new CreateUserPayload(
                    Id: userId,
                    IsDeactivated: false,
                    PrimaryParty: primaryParty ?? string.Empty,
                    IdentityProviderId: string.Empty),
                Rights: Array.Empty<UserRight>())),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("POST {Uri} (userId: {UserId}, primaryParty: {PrimaryParty})",
            new Uri(_httpClient.BaseAddress!, UsersPath), userId, primaryParty ?? "<none>");

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"POST {UsersPath} (userId={userId}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        var payload = await response.Content
            .ReadFromJsonAsync<CreateUserResponse>(cancellationToken: cancellationToken)
            .ConfigureAwait(false)
            ?? throw new JsonLedgerApiException(
                $"POST {UsersPath} returned an empty body.",
                response.StatusCode,
                string.Empty);

        if (payload.User is null || string.IsNullOrWhiteSpace(payload.User.Id))
        {
            throw new JsonLedgerApiException(
                $"POST {UsersPath} returned a response with no user.id field.",
                response.StatusCode,
                string.Empty);
        }

        _logger.LogInformation("Created ledger user {UserId}", payload.User.Id);
        return payload.User.Id;
    }

    private async Task PostGrantRightsAsync(
        string token,
        string userId,
        IReadOnlyList<string> actAs,
        IReadOnlyList<string> readAs,
        CancellationToken cancellationToken)
    {
        var rights = new List<UserRight>(actAs.Count + readAs.Count);
        foreach (var party in actAs)
        {
            rights.Add(UserRight.CanActAs(party));
        }
        foreach (var party in readAs)
        {
            rights.Add(UserRight.CanReadAs(party));
        }

        var path = $"{UsersPath}/{Uri.EscapeDataString(userId)}/rights";
        using var request = new HttpRequestMessage(HttpMethod.Post, path)
        {
            Content = JsonContent.Create(new GrantRightsRequest(userId, string.Empty, rights)),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("POST {Uri} (actAs: {ActAs}, readAs: {ReadAs})",
            new Uri(_httpClient.BaseAddress!, path), actAs.Count, readAs.Count);

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            var body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            throw new JsonLedgerApiException(
                $"POST {path} (userId={userId}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        _logger.LogInformation("Granted {ActAs} actAs / {ReadAs} readAs rights to user {UserId}",
            actAs.Count, readAs.Count, userId);
    }

    private sealed record CreateUserRequest(
        [property: JsonPropertyName("user")] CreateUserPayload User,
        [property: JsonPropertyName("rights")] IReadOnlyList<UserRight> Rights);

    private sealed record CreateUserPayload(
        [property: JsonPropertyName("id")] string Id,
        [property: JsonPropertyName("isDeactivated")] bool IsDeactivated,
        [property: JsonPropertyName("primaryParty")] string PrimaryParty,
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId);

    private sealed record CreateUserResponse(
        [property: JsonPropertyName("user")] CreatedUser? User);

    private sealed record CreatedUser(
        [property: JsonPropertyName("id")] string Id);

    private sealed record GrantRightsRequest(
        [property: JsonPropertyName("userId")] string UserId,
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId,
        [property: JsonPropertyName("rights")] IReadOnlyList<UserRight> Rights);

    private sealed record UserRight(
        [property: JsonPropertyName("kind")] UserRightKind Kind)
    {
        public static UserRight CanActAs(string party) =>
            new(new UserRightKind(CanActAs: new UserRightParty(new UserRightPartyValue(party))));

        public static UserRight CanReadAs(string party) =>
            new(new UserRightKind(CanReadAs: new UserRightParty(new UserRightPartyValue(party))));
    }

    private sealed record UserRightKind(
        [property: JsonPropertyName("CanActAs")] UserRightParty? CanActAs = null,
        [property: JsonPropertyName("CanReadAs")] UserRightParty? CanReadAs = null);

    private sealed record UserRightParty(
        [property: JsonPropertyName("value")] UserRightPartyValue Value);

    private sealed record UserRightPartyValue(
        [property: JsonPropertyName("party")] string Party);
}
