// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

/// <summary>
/// End-to-end smoke tests that boot a <see cref="LocalnetFixture"/> against a
/// running Canton LocalNet and exercise the fixture's core deep modules:
/// participant id lookup, DAR upload, party allocation, user creation. Tagged
/// <c>Integration</c> so they're filtered out of the default unit test run;
/// each test also self-skips when the required env vars are not set.
/// </summary>
[Trait("Category", "Integration")]
public class LocalNetIntegrationTests
{
    private const string DarPathEnv = "CANTON_LOCALNET_TEST_DAR_PATH";

    private const string SkipMessage =
        "Skipping: set CANTON_LOCALNET_<SLOT>_JSON_API_URL, "
        + "CANTON_LOCALNET_<SLOT>_CLIENT_ID, CANTON_LOCALNET_<SLOT>_CLIENT_SECRET "
        + "(per-slot, e.g. A_VALIDATOR_1 / B_VALIDATOR_1) — or the equivalent "
        + "legacy un-namespaced CANTON_LOCALNET_* globals for the fixture's "
        + "default slot — to run the LocalNet smoke tests against a live stack.";

    private const string MultiSlotSkipMessage =
        "Skipping multi-validator smoke: requires both a-validator-1 and "
        + "b-validator-1 reachable. Set CANTON_LOCALNET_A_VALIDATOR_1_* and "
        + "CANTON_LOCALNET_B_VALIDATOR_1_* (or bring up the 5-validator compose "
        + "stack with `make up`) to run this assertion.";

    private const string DarSkipMessage =
        "Set CANTON_LOCALNET_TEST_DAR_PATH to a path pointing at a Canton DAR to run the upload smoke test.";

    [Fact]
    public async Task LocalnetFixture_returns_non_empty_participant_id()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();

        var participantId = await fixture.GetParticipantIdAsync();

        Assert.False(string.IsNullOrWhiteSpace(participantId), "Expected a non-empty participantId from the JSON Ledger API.");
    }

    [Fact]
    public async Task LocalnetFixture_returns_distinct_participant_ids_per_validator_slot()
    {
        if (!EndpointDiscovery.IsSlotAvailable(LocalnetProfile.AValidator1)
            || !EndpointDiscovery.IsSlotAvailable(LocalnetProfile.BValidator1))
        {
            Assert.Skip(MultiSlotSkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();

        var a = await fixture.Validator("a-validator-1").GetParticipantIdAsync();
        var b = await fixture.Validator("b-validator-1").GetParticipantIdAsync();

        Assert.False(string.IsNullOrWhiteSpace(a), "a-validator-1 participantId is empty");
        Assert.False(string.IsNullOrWhiteSpace(b), "b-validator-1 participantId is empty");
        Assert.NotEqual(a, b);
    }

    [Fact]
    public async Task LocalnetFixture_uploads_dar_allocates_party_and_creates_user_with_actAs()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        var darPath = Environment.GetEnvironmentVariable(DarPathEnv);
        if (string.IsNullOrWhiteSpace(darPath) || !File.Exists(darPath))
        {
            Assert.Skip(DarSkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();

        var darOutcome = await fixture.UploadDarAsync(darPath);
        Assert.True(
            darOutcome is DarUploadOutcome.Uploaded or DarUploadOutcome.AlreadyKnown,
            $"Unexpected DAR upload outcome: {darOutcome}");

        var party = await fixture.AllocatePartyAsync("globex");
        Assert.False(string.IsNullOrWhiteSpace(party.PartyId), "Allocated party id should be non-empty.");
        Assert.StartsWith($"globex-{fixture.PartyAllocator.InstanceSuffix}", party.PartyId);

        var userId = $"globex-user-{fixture.PartyAllocator.InstanceSuffix}";
        var createdUserId = await fixture.CreateUserAsync(
            userId,
            primaryParty: party.PartyId,
            actAs: new[] { party.PartyId });

        Assert.Equal(userId, createdUserId);
    }
}
