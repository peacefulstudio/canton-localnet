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
    public Task<IReadOnlyList<ConnectedSynchronizer>> GetConnectedSynchronizersAsync(string party, CancellationToken cancellationToken = default)
        => AdminClient.GetConnectedSynchronizersAsync(party, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="JsonLedgerAdminClient.GetAppSynchronizerIdAsync"/>.</summary>
    public Task<string> GetAppSynchronizerIdAsync(string party, CancellationToken cancellationToken = default)
        => AdminClient.GetAppSynchronizerIdAsync(party, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="DarUploader.UploadAsync(string, CancellationToken)"/>.</summary>
    public Task<DarUploadOutcome> UploadDarAsync(string darPath, CancellationToken cancellationToken = default)
        => DarUploader.UploadAsync(darPath, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="DarUploader.UploadManyAsync"/>.</summary>
    public Task<IReadOnlyList<DarUploadResult>> UploadDarsAsync(
        IEnumerable<string> darPaths,
        CancellationToken cancellationToken = default)
        => DarUploader.UploadManyAsync(darPaths, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="PartyAllocator.AllocateAsync"/>.</summary>
    public Task<AllocatedParty> AllocatePartyAsync(
        string consumerPrefix,
        string? displayName = null,
        CancellationToken cancellationToken = default)
        => PartyAllocator.AllocateAsync(consumerPrefix, displayName, cancellationToken);

    /// <summary>Convenience pass-through to <see cref="UserBuilder.CreateAsync"/>.</summary>
    public Task<string> CreateUserAsync(
        string userId,
        string? primaryParty = null,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.CreateAsync(userId, primaryParty, actAs, readAs, cancellationToken);

    /// <summary>
    /// Convenience pass-through to <see cref="UserBuilder.GrantRightsAsync"/>.
    /// Grants <c>CanActAs</c> (and optionally <c>CanReadAs</c>) for the given
    /// parties to an already-existing ledger user — e.g. this slot's validator
    /// service-account token user (<see cref="ValidatorUserId"/>) — so that
    /// <c>client_credentials</c>-authenticated command submission is authorized.
    /// </summary>
    public Task GrantUserRightsAsync(
        string userId,
        IEnumerable<string>? actAs = null,
        IEnumerable<string>? readAs = null,
        CancellationToken cancellationToken = default)
        => UserBuilder.GrantRightsAsync(userId, actAs, readAs, cancellationToken);

    /// <summary>
    /// Disposes the scoped service container (if any) created for this
    /// view. When this view is the fixture's default slot, the container
    /// is owned by <see cref="LocalnetFixture"/> and this method is a
    /// no-op.
    /// </summary>
    public async ValueTask DisposeAsync()
    {
        if (_ownedServices is not null)
        {
            TokenProvider.Dispose();
            await _ownedServices.DisposeAsync().ConfigureAwait(false);
        }
    }
}
