// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Per-slot view of a <see cref="LocalnetFixture"/>. Exposes the same deep
/// modules (<see cref="JsonLedgerAdminClient"/>, <see cref="DarUploader"/>,
/// <see cref="PartyAllocator"/>, <see cref="UserBuilder"/>) but scoped to
/// a single validator slot, so tests that span multiple validators don't
/// have to juggle separate top-level fixtures.
///
/// Obtain one via <see cref="LocalnetFixture.Validator(string)"/> or
/// <see cref="LocalnetFixture.Validator(LocalnetProfile)"/>. The first call
/// for a given slot resolves endpoints and constructs the underlying
/// clients; subsequent calls return the cached instance.
/// </summary>
public sealed class ValidatorFixture : IAsyncDisposable
{
    private readonly ServiceProvider? _ownedServices;
    private readonly CreatedUsers _createdUsers = new();
    private int _servicesDisposed;

    internal ValidatorFixture(
        string slot,
        LocalnetProfile profile,
        LocalnetEndpoints endpoints,
        JsonLedgerAdminClient adminClient,
        OAuth2TokenProvider tokenProvider,
        DarUploader darUploader,
        PartyAllocator partyAllocator,
        UserBuilder userBuilder,
        ServiceProvider? ownedServices)
    {
        Slot = slot;
        Profile = profile;
        Endpoints = endpoints;
        AdminClient = adminClient;
        TokenProvider = tokenProvider;
        DarUploader = darUploader;
        PartyAllocator = partyAllocator;
        UserBuilder = userBuilder;
        _ownedServices = ownedServices;
    }

    /// <summary>
    /// Canonical slot name this view is bound to (e.g. "a-validator-1").
    /// </summary>
    public string Slot { get; }

    /// <summary>
    /// Resolved <see cref="LocalnetProfile"/> for this slot.
    /// </summary>
    public LocalnetProfile Profile { get; }

    /// <summary>
    /// Resolved endpoint set for this validator slot.
    /// </summary>
    public LocalnetEndpoints Endpoints { get; }

    /// <summary>
    /// The ledger user id this slot's <c>client_credentials</c> token
    /// authenticates as. Grant this user <c>CanActAs</c> (via
    /// <see cref="GrantUserRightsAsync"/>) before submitting commands as an
    /// allocated party. See <see cref="LocalnetEndpoints.ValidatorUserId"/>.
    /// </summary>
    public string ValidatorUserId => Endpoints.ValidatorUserId;

    /// <summary>JSON Ledger Admin client scoped to this slot.</summary>
    public JsonLedgerAdminClient AdminClient { get; }

    /// <summary>OAuth2 token provider scoped to this slot's realm.</summary>
    public OAuth2TokenProvider TokenProvider { get; }

    /// <summary>DAR uploader scoped to this slot.</summary>
    public DarUploader DarUploader { get; }

    /// <summary>Party allocator scoped to this slot.</summary>
    public PartyAllocator PartyAllocator { get; }

    /// <summary>User builder scoped to this slot.</summary>
    public UserBuilder UserBuilder { get; }

    /// <summary>Convenience pass-through to <see cref="JsonLedgerAdminClient.GetParticipantIdAsync"/>.</summary>
    public Task<string> GetParticipantIdAsync(CancellationToken cancellationToken = default)
        => AdminClient.GetParticipantIdAsync(cancellationToken);

    /// <summary>Convenience pass-through to <see cref="JsonLedgerAdminClient.GetConnectedSynchronizersAsync"/>.</summary>
    public Task<IReadOnlyList<ConnectedSynchronizer>> GetConnectedSynchronizersAsync(string? party = null, CancellationToken cancellationToken = default)
        => AdminClient.GetConnectedSynchronizersAsync(party, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="JsonLedgerAdminClient.GetAppSynchronizerIdAsync"/>.</summary>
    public Task<string> GetAppSynchronizerIdAsync(string? party = null, CancellationToken cancellationToken = default)
        => AdminClient.GetAppSynchronizerIdAsync(party, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="DarUploader.UploadAsync(string, CancellationToken)"/>.</summary>
    public Task<DarUploadOutcome> UploadDarAsync(string darPath, CancellationToken cancellationToken = default)
        => DarUploader.UploadAsync(darPath, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="DarUploader.UploadManyAsync"/>.</summary>
    public Task<IReadOnlyList<DarUploadResult>> UploadDarsAsync(
        IEnumerable<string> darPaths,
        CancellationToken cancellationToken = default)
        => DarUploader.UploadManyAsync(darPaths, cancellationToken);

    /// <summary>
    /// Convenience pass-through to
    /// <see cref="PartyAllocator.AllocateAsync(string, CancellationToken)"/>.
    /// </summary>
    public Task<AllocatedParty> AllocatePartyAsync(
        string consumerPrefix,
        CancellationToken cancellationToken = default)
        => PartyAllocator.AllocateAsync(consumerPrefix, cancellationToken);

    /// <summary>
    /// Convenience pass-through to
    /// <see cref="PartyAllocator.AllocateOnSynchronizerAsync(string, string, CancellationToken)"/>.
    /// </summary>
    public Task<AllocatedParty> AllocatePartyOnSynchronizerAsync(
        string consumerPrefix,
        string synchronizerId,
        CancellationToken cancellationToken = default)
        => PartyAllocator.AllocateOnSynchronizerAsync(consumerPrefix, synchronizerId, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.CreateAsync"/> that
    /// also records the created user, so <see cref="DisposeAsync"/> deletes it.
    /// Dispose every <see cref="UserRightsLease"/> on the user first.
    /// </summary>
    public async Task<string> CreateUserAsync(
        string userId,
        string? primaryParty = null,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
    {
        var created = await UserBuilder.CreateAsync(userId, primaryParty, actAs, readAs, cancellationToken).ConfigureAwait(false);
        _createdUsers.Record(created);
        return created;
    }

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.GrantRightsAsync"/>.
    /// Grants <c>CanActAs</c> (and optionally <c>CanReadAs</c>) for the given
    /// parties to an already-existing ledger user — e.g. this slot's validator
    /// service-account token user (<see cref="ValidatorUserId"/>) — so that
    /// <c>client_credentials</c>-authenticated command submission is authorized.
    /// The rights stay until something revokes them, and a participant caps a
    /// user at 1000 of them; prefer <see cref="GrantUserRightsLeaseAsync"/>,
    /// which hands them back on dispose.
    /// </summary>
    public Task GrantUserRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.GrantRightsAsync(userId, actAs, readAs, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.GrantRightsLeaseAsync"/>.
    /// Grants the rights and returns a lease that revokes exactly what it newly
    /// granted, on <see cref="CancellationToken.None"/>, when disposed —
    /// throwing from <see cref="UserRightsLease.DisposeAsync"/> if the
    /// participant does not confirm the hand-back.
    /// </summary>
    public Task<UserRightsLease> GrantUserRightsLeaseAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.GrantRightsLeaseAsync(userId, actAs, readAs, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.RevokeRightsAsync"/>.
    /// Revokes the given <c>CanActAs</c>/<c>CanReadAs</c> rights from a ledger
    /// user, throwing unless the participant reports every one of them as newly
    /// revoked — so it is not safe to call twice for the same parties. For
    /// teardown use <see cref="GrantUserRightsLeaseAsync"/>.
    /// </summary>
    public Task RevokeUserRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.RevokeRightsAsync(userId, actAs, readAs, cancellationToken);

    /// <summary>
    /// Deletes the ledger users created through <see cref="CreateUserAsync"/>,
    /// then disposes the scoped service container (if any) created for this
    /// view. When this view is the fixture's default slot, the container
    /// is owned by <see cref="LocalnetFixture"/> and it is left alone.
    /// <para>
    /// A failed delete throws and leaves the container alive, so calling this
    /// again retries the users still outstanding; see
    /// <see cref="LocalnetFixture.DisposeAsync"/> for the attempt rules.
    /// </para>
    /// </summary>
    /// <exception cref="JsonLedgerApiException">
    /// A single delete failed. When several did, an
    /// <see cref="AggregateException"/> carries each of them.
    /// </exception>
    public async ValueTask DisposeAsync()
    {
        await _createdUsers.DeleteAllAsync(UserBuilder).ConfigureAwait(false);
        if (_ownedServices is not null && Interlocked.Exchange(ref _servicesDisposed, 1) == 0)
        {
            TokenProvider.Dispose();
            await _ownedServices.DisposeAsync().ConfigureAwait(false);
        }
    }
}
