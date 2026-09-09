// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Creates Canton ledger users (<c>POST /v2/users</c>) and grants, leases or
/// revokes <c>CanActAs</c>/<c>CanReadAs</c> rights
/// (<c>POST</c>/<c>PATCH /v2/users/{id}/rights</c>) using bearer tokens from
/// <see cref="OAuth2TokenProvider"/>.
/// </summary>
public sealed class UserBuilder
{
    private const string UsersPath = "v2/users";
    private const string CanActAsKind = "CanActAs";
    private const string CanReadAsKind = "CanReadAs";

    private readonly HttpClient _httpClient;
    private readonly OAuth2TokenProvider _tokenProvider;
    private readonly ILogger<UserBuilder> _logger;

    /// <summary>
    /// Creates a builder bound to a JSON Ledger API <see cref="HttpClient"/>.
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
    /// as reported by the participant. Duplicate parties within
    /// <paramref name="actAs"/> or within <paramref name="readAs"/> are
    /// collapsed; the same party in both is two distinct rights.
    /// </summary>
    /// <exception cref="ArgumentException"><paramref name="userId"/> is empty.</exception>
    /// <exception cref="JsonLedgerApiException">
    /// The participant rejected the request, or returned no <c>user.id</c>.
    /// </exception>
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

        var rights = BuildRights(actAs, readAs);
        if (rights.Count > 0)
        {
            await PostGrantRightsAsync(token, created, rights, cancellationToken).ConfigureAwait(false);
        }

        return created;
    }

    /// <summary>
    /// Grants <c>CanActAs</c> (and optionally <c>CanReadAs</c>) rights to an
    /// already-existing ledger user via <c>POST /v2/users/{id}/rights</c>,
    /// without creating the user first. Use this to authorize the validator's
    /// service-account token user (see
    /// <see cref="LocalnetEndpoints.ValidatorUserId"/>) to act as an allocated
    /// party so that <c>client_credentials</c>-authenticated command submission
    /// passes Canton's Authorizer check. No request is issued when both
    /// <paramref name="actAs"/> and <paramref name="readAs"/> are empty.
    /// <para>
    /// Rights granted this way stay on the user until something revokes them,
    /// and a participant caps a user at 1000 rights. On a long-lived shared
    /// LocalNet, prefer <see cref="GrantRightsLeaseAsync"/>, which hands the
    /// rights back on dispose.
    /// </para>
    /// </summary>
    /// <exception cref="ArgumentException"><paramref name="userId"/> is empty.</exception>
    /// <exception cref="JsonLedgerApiException">The participant rejected the grant.</exception>
    public async Task GrantRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(userId))
        {
            throw new ArgumentException("User id must be non-empty.", nameof(userId));
        }

        var rights = BuildRights(actAs, readAs);
        if (rights.Count == 0)
        {
            return;
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        await PostGrantRightsAsync(token, userId, rights, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Revokes rights from an existing ledger user via
    /// <c>PATCH /v2/users/{id}/rights</c>. No request is issued when both
    /// <paramref name="actAs"/> and <paramref name="readAs"/> are empty.
    /// <para>
    /// This is the <em>strict</em> counterpart of
    /// <see cref="GrantRightsAsync"/>: it throws unless the participant reports
    /// every requested right as newly revoked. A right that was already absent
    /// counts as a shortfall, so calling it twice for the same parties throws
    /// the second time. It is a statement about the rights you hold, not an
    /// idempotent "make sure these are gone".
    /// </para>
    /// <para>
    /// It is therefore the wrong tool for teardown — use
    /// <see cref="GrantRightsLeaseAsync"/>, which knows what it granted and
    /// hands exactly that back. If you do call this directly and it reports a
    /// partial revoke, a blind retry throws again on the rights that did come
    /// back: narrow the request to the outstanding parties yourself. The
    /// participant's <c>newlyRevokedRights</c> is on
    /// <see cref="JsonLedgerApiException.ResponseBody"/>, so the remainder is
    /// computable from the exception.
    /// </para>
    /// </summary>
    /// <exception cref="ArgumentException"><paramref name="userId"/> is empty.</exception>
    /// <exception cref="JsonLedgerApiException">
    /// The participant returned a non-success status, or reported fewer newly
    /// revoked rights than were requested.
    /// </exception>
    public async Task RevokeRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(userId))
        {
            throw new ArgumentException("User id must be non-empty.", nameof(userId));
        }

        var rights = BuildRights(actAs, readAs);
        if (rights.Count == 0)
        {
            return;
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        var outcome = await PatchRevokeRightsAsync(token, userId, rights, cancellationToken).ConfigureAwait(false);
        if (outcome.Revoked.Count < rights.Count)
        {
            throw PartialRevokeError(userId, outcome, rights.Count);
        }
    }

    /// <summary>
    /// Grants the listed rights and returns a <see cref="UserRightsLease"/> that
    /// revokes them again when disposed, so a test that runs to completion
    /// cannot leave rights behind on a shared participant. No request is issued,
    /// and the returned lease owns nothing, when both <paramref name="actAs"/>
    /// and <paramref name="readAs"/> are empty.
    /// <para>
    /// The lease revokes exactly the rights the participant reported as
    /// <c>newlyGrantedRights</c>, never the full requested set: a right that was
    /// already on the user belongs to whoever granted it first, and handing it
    /// back here would break them. A lease that newly granted nothing therefore
    /// owns nothing — see <see cref="UserRightsLease.Rights"/> — and disposing
    /// it is a no-op.
    /// </para>
    /// <para>
    /// <paramref name="cancellationToken"/> governs the run-up to the grant (the
    /// OAuth2 token fetch) and nothing after it: the grant itself is issued on
    /// <see cref="CancellationToken.None"/>, because a cancellation landing
    /// between the participant committing the rights and this method returning
    /// would strand them with no lease to hand them back. The grant is bounded
    /// by <see cref="HttpClient.Timeout"/> instead.
    /// </para>
    /// </summary>
    /// <exception cref="ArgumentException"><paramref name="userId"/> is empty.</exception>
    /// <exception cref="JsonLedgerApiException">The participant rejected the grant.</exception>
    /// <exception cref="UserRightsGrantedWithoutLeaseException">
    /// The grant succeeded but its response carried no readable
    /// <c>newlyGrantedRights</c>, leaving the lease unable to tell what it owns.
    /// The rights are on the participant; the exception carries the requested
    /// parties so an outer teardown can act on them.
    /// </exception>
    public async Task<UserRightsLease> GrantRightsLeaseAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(userId))
        {
            throw new ArgumentException("User id must be non-empty.", nameof(userId));
        }

        var actAsParties = DistinctParties(actAs);
        var readAsParties = DistinctParties(readAs);
        var requested = BuildRights(actAsParties, readAsParties);
        if (requested.Count == 0)
        {
            return new UserRightsLease(this, userId, Array.Empty<JsonElement>());
        }

        var token = await _tokenProvider.GetAccessTokenAsync(cancellationToken).ConfigureAwait(false);
        var response = await PostGrantRightsAsync(token, userId, requested, CancellationToken.None).ConfigureAwait(false);
        var granted = ReadRights(response.Body, "newlyGrantedRights")
            ?? throw new UserRightsGrantedWithoutLeaseException(
                userId,
                actAsParties,
                readAsParties,
                response.StatusCode,
                response.Body);

        return new UserRightsLease(this, userId, granted);
    }

    internal ValueTask<string> GetTokenAsync(CancellationToken cancellationToken)
        => _tokenProvider.GetAccessTokenAsync(cancellationToken);

    internal async Task<RevokeOutcome> PatchRevokeRightsAsync(
        string token,
        string userId,
        IReadOnlyList<JsonElement> rights,
        CancellationToken cancellationToken)
    {
        var path = RightsPath(userId);
        using var request = new HttpRequestMessage(HttpMethod.Patch, path)
        {
            Content = JsonContent.Create(new UserRightsRequest(userId, string.Empty, rights)),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("PATCH {Uri} ({Rights} rights)",
            new Uri(_httpClient.BaseAddress!, path), rights.Count);

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        var body = await ReadBodyOfCommittedRequestAsync(response).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            throw new JsonLedgerApiException(
                $"PATCH {path} (userId={userId}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        var revoked = ReadRights(body, "newlyRevokedRights") ?? Array.Empty<JsonElement>();
        _logger.LogInformation("Revoked {Rights} rights from user {UserId}", revoked.Count, userId);
        return new RevokeOutcome(revoked, response.StatusCode, body, path);
    }

    internal JsonLedgerApiException PartialRevokeError(string userId, RevokeOutcome outcome, int requested)
        => new(
            $"PATCH {outcome.Path} (userId={userId}) reported {outcome.Revoked.Count} of {requested} "
            + $"requested rights as newly revoked; the other {requested - outcome.Revoked.Count} "
            + $"were not reported as revoked and may still be granted: {outcome.Body}",
            outcome.StatusCode,
            outcome.Body);

    internal async Task<IReadOnlyList<JsonElement>> RightsStillHeldAsync(
        string token,
        string userId,
        IReadOnlyList<JsonElement> candidates,
        CancellationToken cancellationToken)
    {
        var path = RightsPath(userId);
        using var request = new HttpRequestMessage(HttpMethod.Get, path);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("GET {Uri}", new Uri(_httpClient.BaseAddress!, path));

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            return candidates;
        }

        var body = await ReadBodyOfCommittedRequestAsync(response).ConfigureAwait(false);
        var held = ReadRights(body, "rights");
        if (held is null)
        {
            return candidates;
        }

        var identities = held.Select(RightIdentity).ToHashSet(StringComparer.Ordinal);
        return candidates.Where(right => identities.Contains(RightIdentity(right))).ToArray();
    }

    internal static (IReadOnlyList<string> ActAs, IReadOnlyList<string> ReadAs) ClassifyParties(
        IReadOnlyList<JsonElement> rights)
    {
        var actAs = new List<string>();
        var readAs = new List<string>();
        foreach (var right in rights)
        {
            if (!TryClassify(right, out var kind, out var party))
            {
                continue;
            }
            (kind == CanActAsKind ? actAs : readAs).Add(party);
        }
        return (actAs, readAs);
    }

    internal static IReadOnlyList<JsonElement> Outstanding(
        IReadOnlyList<JsonElement> owned,
        IReadOnlyList<JsonElement> settled)
    {
        var identities = settled.Select(RightIdentity).ToHashSet(StringComparer.Ordinal);
        return owned.Where(right => !identities.Contains(RightIdentity(right))).ToArray();
    }

    private static string RightsPath(string userId) =>
        $"{UsersPath}/{Uri.EscapeDataString(userId)}/rights";

    private static IReadOnlyList<string> DistinctParties(IEnumerable<string>? parties)
        => (parties ?? Array.Empty<string>()).Distinct(StringComparer.Ordinal).ToArray();

    private static IReadOnlyList<JsonElement> BuildRights(
        IEnumerable<string>? actAs,
        IEnumerable<string>? readAs)
    {
        var rights = new List<JsonElement>();
        foreach (var party in DistinctParties(actAs))
        {
            rights.Add(JsonSerializer.SerializeToElement(UserRight.CanActAs(party)));
        }
        foreach (var party in DistinctParties(readAs))
        {
            rights.Add(JsonSerializer.SerializeToElement(UserRight.CanReadAs(party)));
        }
        return rights;
    }

    private static IReadOnlyList<JsonElement>? ReadRights(string body, string field)
    {
        if (string.IsNullOrWhiteSpace(body))
        {
            return null;
        }

        JsonDocument document;
        try
        {
            document = JsonDocument.Parse(body);
        }
        catch (JsonException)
        {
            return null;
        }

        using (document)
        {
            if (document.RootElement.ValueKind != JsonValueKind.Object
                || !document.RootElement.TryGetProperty(field, out var array)
                || array.ValueKind != JsonValueKind.Array)
            {
                return null;
            }

            var rights = new List<JsonElement>(array.GetArrayLength());
            foreach (var right in array.EnumerateArray())
            {
                rights.Add(right.Clone());
            }
            return rights;
        }
    }

    private static string RightIdentity(JsonElement right)
        => TryClassify(right, out var kind, out var party) ? $"{kind} {party}" : right.GetRawText();

    private static bool TryClassify(JsonElement right, out string kind, out string party)
    {
        kind = string.Empty;
        party = string.Empty;
        if (right.ValueKind != JsonValueKind.Object
            || !right.TryGetProperty("kind", out var kinds)
            || kinds.ValueKind != JsonValueKind.Object)
        {
            return false;
        }

        return TryClassifyAs(kinds, CanActAsKind, ref kind, ref party)
            || TryClassifyAs(kinds, CanReadAsKind, ref kind, ref party);
    }

    private static bool TryClassifyAs(JsonElement kinds, string candidate, ref string kind, ref string party)
    {
        if (!kinds.TryGetProperty(candidate, out var value)
            || value.ValueKind != JsonValueKind.Object
            || !value.TryGetProperty("value", out var partyValue)
            || partyValue.ValueKind != JsonValueKind.Object
            || !partyValue.TryGetProperty("party", out var partyId)
            || partyId.ValueKind != JsonValueKind.String)
        {
            return false;
        }

        kind = candidate;
        party = partyId.GetString() ?? string.Empty;
        return true;
    }

    private static Task<string> ReadBodyOfCommittedRequestAsync(HttpResponseMessage response)
        => response.Content.ReadAsStringAsync(CancellationToken.None);

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
                Rights: Array.Empty<JsonElement>())),
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

    private async Task<LedgerResponse> PostGrantRightsAsync(
        string token,
        string userId,
        IReadOnlyList<JsonElement> rights,
        CancellationToken cancellationToken)
    {
        var path = RightsPath(userId);
        using var request = new HttpRequestMessage(HttpMethod.Post, path)
        {
            Content = JsonContent.Create(new UserRightsRequest(userId, string.Empty, rights)),
        };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _logger.LogDebug("POST {Uri} ({Rights} rights)",
            new Uri(_httpClient.BaseAddress!, path), rights.Count);

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        var body = await ReadBodyOfCommittedRequestAsync(response).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            throw new JsonLedgerApiException(
                $"POST {path} (userId={userId}) returned {(int)response.StatusCode} {response.ReasonPhrase}: {body}",
                response.StatusCode,
                body);
        }

        _logger.LogInformation("Granted {Rights} rights to user {UserId}", rights.Count, userId);
        return new LedgerResponse(response.StatusCode, body);
    }

    internal sealed record LedgerResponse(HttpStatusCode StatusCode, string Body);

    internal sealed record RevokeOutcome(
        IReadOnlyList<JsonElement> Revoked,
        HttpStatusCode StatusCode,
        string Body,
        string Path);

    private sealed record CreateUserRequest(
        [property: JsonPropertyName("user")] CreateUserPayload User,
        [property: JsonPropertyName("rights")] IReadOnlyList<JsonElement> Rights);

    private sealed record CreateUserPayload(
        [property: JsonPropertyName("id")] string Id,
        [property: JsonPropertyName("isDeactivated")] bool IsDeactivated,
        [property: JsonPropertyName("primaryParty")] string PrimaryParty,
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId);

    private sealed record CreateUserResponse(
        [property: JsonPropertyName("user")] CreatedUser? User);

    private sealed record CreatedUser(
        [property: JsonPropertyName("id")] string Id);

    private sealed record UserRightsRequest(
        [property: JsonPropertyName("userId")] string UserId,
        [property: JsonPropertyName("identityProviderId")] string IdentityProviderId,
        [property: JsonPropertyName("rights")] IReadOnlyList<JsonElement> Rights);

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

/// <summary>
/// Rights held on a ledger user for the lifetime of the lease, taken by
/// <see cref="UserBuilder.GrantRightsLeaseAsync"/> and handed back by
/// <see cref="DisposeAsync"/>.
/// <para>
/// A lease owns only the rights the participant reported as newly granted, so
/// two overlapping leases on the same party do not both own it: the first
/// grantor owns it, and a second lease taken while the first is alive owns
/// nothing and reports an empty <see cref="Rights"/>. That second holder's
/// authorization therefore ends when the <em>first</em> lease is disposed, not
/// when its own is — check <see cref="Rights"/> if you need to know whether you
/// are the owner. The alternative, where the second lease revokes on dispose,
/// would take the rights out from under the first holder with certainty rather
/// than by coincidence.
/// </para>
/// </summary>
public sealed class UserRightsLease : IAsyncDisposable
{
    private readonly UserBuilder _builder;
    private IReadOnlyList<JsonElement> _owned;
    private int _disposed;
    private int _attempts;

    internal UserRightsLease(UserBuilder builder, string userId, IReadOnlyList<JsonElement> owned)
    {
        _builder = builder;
        UserId = userId;
        _owned = owned;
        Own(owned);
    }

    /// <summary>Ledger user the leased rights sit on.</summary>
    public string UserId { get; }

    /// <summary>
    /// The rights this lease owns and will revoke, exactly as the participant
    /// reported them, one raw JSON object per right. Empty when the grant newly
    /// granted nothing — every requested right was already on the user — in
    /// which case disposal is a no-op. Narrows to the outstanding rights when a
    /// revoke comes back partial.
    /// </summary>
    public IReadOnlyList<string> Rights { get; private set; } = Array.Empty<string>();

    /// <summary>
    /// Parties of the owned <c>CanActAs</c> rights. A right whose shape this
    /// package does not model appears in <see cref="Rights"/> only.
    /// </summary>
    public IReadOnlyList<string> ActAs { get; private set; } = Array.Empty<string>();

    /// <summary>
    /// Parties of the owned <c>CanReadAs</c> rights. A right whose shape this
    /// package does not model appears in <see cref="Rights"/> only.
    /// </summary>
    public IReadOnlyList<string> ReadAs { get; private set; } = Array.Empty<string>();

    /// <summary>
    /// Revokes the owned rights on <see cref="CancellationToken.None"/>, so a
    /// run cancelled mid-flight still hands them back. Disposing a lease that
    /// owns nothing issues no request.
    /// <para>
    /// A revoke that succeeded is never repeated. One that fails throws — a
    /// silently swallowed hand-back is a permanent, invisible leak on a shared
    /// participant — and leaves the lease disposable again, narrowed to the
    /// rights the participant did not confirm, so the hand-back can be retried:
    /// </para>
    /// <code>
    /// var lease = await fixture.GrantUserRightsLeaseAsync(userId, actAs: parties);
    /// try { /* test body */ }
    /// finally
    /// {
    ///     try { await lease.DisposeAsync(); }
    ///     catch (JsonLedgerApiException) { await lease.DisposeAsync(); }
    /// }
    /// </code>
    /// <para>
    /// Prefer that shape over <c>await using</c> when the test body has
    /// assertions of its own: <c>await using</c> compiles to a
    /// <c>try</c>/<c>finally</c>, and an exception thrown from the <c>finally</c>
    /// replaces the in-flight one, so a failing assertion followed by a failing
    /// revoke reports the revoke. It also disposes exactly once, so the retry
    /// above is unreachable through it.
    /// </para>
    /// </summary>
    /// <exception cref="JsonLedgerApiException">
    /// The participant rejected the revoke, or did not report every owned right
    /// as revoked. The message names the user and the outstanding rights, so it
    /// does not read as a failure of the test body it interrupted.
    /// </exception>
    public async ValueTask DisposeAsync()
    {
        if (Interlocked.Exchange(ref _disposed, 1) != 0 || _owned.Count == 0)
        {
            return;
        }

        var attempt = Interlocked.Increment(ref _attempts);
        try
        {
            var token = await _builder.GetTokenAsync(CancellationToken.None).ConfigureAwait(false);
            var outcome = await _builder
                .PatchRevokeRightsAsync(token, UserId, _owned, CancellationToken.None)
                .ConfigureAwait(false);
            if (outcome.Revoked.Count >= _owned.Count)
            {
                return;
            }

            var outstanding = UserBuilder.Outstanding(_owned, outcome.Revoked);
            if (attempt > 1)
            {
                outstanding = await _builder
                    .RightsStillHeldAsync(token, UserId, outstanding, CancellationToken.None)
                    .ConfigureAwait(false);
                if (outstanding.Count == 0)
                {
                    return;
                }
            }

            var requested = _owned.Count;
            Own(outstanding);
            throw _builder.PartialRevokeError(UserId, outcome, requested);
        }
        catch
        {
            Interlocked.Exchange(ref _disposed, 0);
            throw;
        }
    }

    private void Own(IReadOnlyList<JsonElement> rights)
    {
        _owned = rights;
        Rights = rights.Select(right => right.GetRawText()).ToArray();
        (ActAs, ReadAs) = UserBuilder.ClassifyParties(rights);
    }
}

/// <summary>
/// Raised when <see cref="UserBuilder.GrantRightsLeaseAsync"/> granted rights
/// that the participant then failed to describe, leaving no lease to hand them
/// back. The rights are on the ledger user; <see cref="ActAs"/> and
/// <see cref="ReadAs"/> carry the requested parties so an outer teardown has a
/// handle on them.
/// </summary>
public sealed class UserRightsGrantedWithoutLeaseException : JsonLedgerApiException
{
    /// <summary>Creates the exception for a grant that could not be leased.</summary>
    /// <param name="userId">Ledger user the rights were granted to.</param>
    /// <param name="actAs">Requested <c>CanActAs</c> parties.</param>
    /// <param name="readAs">Requested <c>CanReadAs</c> parties.</param>
    /// <param name="statusCode">Status the participant returned for the grant.</param>
    /// <param name="responseBody">Raw grant response, which lacked the field.</param>
    public UserRightsGrantedWithoutLeaseException(
        string userId,
        IReadOnlyList<string> actAs,
        IReadOnlyList<string> readAs,
        HttpStatusCode statusCode,
        string responseBody)
        : base(
            $"Granted {actAs.Count + readAs.Count} right(s) to user {userId}, but the response reported no "
            + "newlyGrantedRights, so no lease could be taken and the rights are now on the participant "
            + $"unowned. Revoke them with RevokeRightsAsync(\"{userId}\", actAs: [{string.Join(", ", actAs)}], "
            + $"readAs: [{string.Join(", ", readAs)}]) — that is the requested set, which may include rights "
            + $"another holder granted first. Response: {responseBody}",
            statusCode,
            responseBody)
    {
        UserId = userId;
        ActAs = actAs;
        ReadAs = readAs;
    }

    /// <summary>Ledger user the rights were granted to.</summary>
    public string UserId { get; }

    /// <summary>Requested <c>CanActAs</c> parties.</summary>
    public IReadOnlyList<string> ActAs { get; }

    /// <summary>Requested <c>CanReadAs</c> parties.</summary>
    public IReadOnlyList<string> ReadAs { get; }
}
