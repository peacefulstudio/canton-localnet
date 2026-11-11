// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

/// <summary>
/// End-to-end smoke tests that boot a <see cref="LocalnetFixture"/> against a
/// running Canton LocalNet and exercise the fixture's core deep modules:
/// participant id lookup, DAR upload, party allocation, user creation, and the
/// user-rights lease — including the raw wire contract the lease depends on.
/// Tagged
/// <c>Integration</c> so they're filtered out of the default unit test run;
/// each test also self-skips when the required env vars are not set.
/// </summary>
[Trait("Category", "Integration")]
[Collection("EnvVarMutation")]
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

    [Theory]
    [InlineData(LocalnetProfile.AValidator1, 11901)]
    [InlineData(LocalnetProfile.BValidator1, 12901)]
    public async Task EndpointDiscovery_grpc_ledger_api_accepts_a_connection_on_the_slot_port(
        LocalnetProfile profile,
        int expectedPort)
    {
        if (!EndpointDiscovery.IsSlotAvailable(profile))
        {
            Assert.Skip(SkipMessage);
        }

        var grpcLedgerApi = EndpointDiscovery.Resolve(profile).GrpcLedgerApi;

        Assert.Equal(expectedPort, grpcLedgerApi.Port);
        using var client = new System.Net.Sockets.TcpClient();
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));
        await client.ConnectAsync(grpcLedgerApi.Host, grpcLedgerApi.Port, timeout.Token);
        Assert.True(client.Connected);
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

    [Fact]
    public async Task JsonLedgerApi_reports_empty_rights_lists_as_a_present_empty_array()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();
        using var http = new HttpClient { BaseAddress = fixture.Endpoints.JsonLedgerApi };
        var token = await fixture.TokenProvider.GetAccessTokenAsync();
        var version = await ParticipantVersionAsync(http, token);

        var party = await fixture.AllocatePartyAsync("encoder");
        var userId = $"encoder-user-{fixture.PartyAllocator.InstanceSuffix}";
        await fixture.CreateUserAsync(userId);
        var rightsPath = $"v2/users/{userId}/rights";
        var actAsParty = RightsPayload(userId, party.PartyId);

        var emptyList = await SendAsync(http, token, HttpMethod.Get, rightsPath, body: null);
        AssertPresentAndEmpty(emptyList.Body, "rights", "a user holding no rights", version);

        var firstGrant = await SendAsync(http, token, HttpMethod.Post, rightsPath, actAsParty);
        Assert.Equal(HttpStatusCode.OK, firstGrant.Status);
        Assert.Equal(1, RightsCount(firstGrant.Body, "newlyGrantedRights"));

        var repeatedGrant = await SendAsync(http, token, HttpMethod.Post, rightsPath, actAsParty);
        Assert.Equal(HttpStatusCode.OK, repeatedGrant.Status);
        AssertPresentAndEmpty(repeatedGrant.Body, "newlyGrantedRights", "a grant that was already held", version);

        var firstRevoke = await SendAsync(http, token, HttpMethod.Patch, rightsPath, actAsParty);
        Assert.Equal(HttpStatusCode.OK, firstRevoke.Status);
        Assert.Equal(1, RightsCount(firstRevoke.Body, "newlyRevokedRights"));

        var repeatedRevoke = await SendAsync(http, token, HttpMethod.Patch, rightsPath, actAsParty);
        Assert.Equal(HttpStatusCode.OK, repeatedRevoke.Status);
        AssertPresentAndEmpty(repeatedRevoke.Body, "newlyRevokedRights", "a revoke of a right already gone", version);
    }

    [Fact]
    public async Task LocalnetFixture_lease_owns_only_the_rights_it_newly_granted()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();
        using var http = new HttpClient { BaseAddress = fixture.Endpoints.JsonLedgerApi };
        var token = await fixture.TokenProvider.GetAccessTokenAsync();

        var party = await fixture.AllocatePartyAsync("lease");
        var userId = $"lease-user-{fixture.PartyAllocator.InstanceSuffix}";
        await fixture.CreateUserAsync(userId);
        var rightsPath = $"v2/users/{userId}/rights";

        var owner = await fixture.GrantUserRightsLeaseAsync(userId, actAs: new[] { party.PartyId });
        Assert.Equal(new[] { party.PartyId }, owner.ActAs);

        var overlapping = await fixture.GrantUserRightsLeaseAsync(userId, actAs: new[] { party.PartyId });
        Assert.Empty(overlapping.Rights);

        await overlapping.DisposeAsync();
        var afterOverlappingDispose = await SendAsync(http, token, HttpMethod.Get, rightsPath, body: null);
        Assert.Equal(1, RightsCount(afterOverlappingDispose.Body, "rights"));

        await owner.DisposeAsync();
        var afterOwnerDispose = await SendAsync(http, token, HttpMethod.Get, rightsPath, body: null);
        Assert.Equal(0, RightsCount(afterOwnerDispose.Body, "rights"));
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_the_users_it_created()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        await using var observer = LocalnetFixture.FromEnvironment();
        using var http = new HttpClient { BaseAddress = observer.Endpoints.JsonLedgerApi };
        var token = await observer.TokenProvider.GetAccessTokenAsync();
        var fixture = LocalnetFixture.FromEnvironment();
        var party = await fixture.AllocatePartyAsync("teardown");
        var userId = $"teardown-user-{fixture.PartyAllocator.InstanceSuffix}";
        await fixture.CreateUserAsync(userId, primaryParty: party.PartyId, actAs: new[] { party.PartyId });
        var userPath = $"v2/users/{userId}";
        var before = await SendAsync(http, token, HttpMethod.Get, userPath, body: null);
        Assert.Equal(HttpStatusCode.OK, before.Status);

        await fixture.DisposeAsync();

        var after = await SendAsync(http, token, HttpMethod.Get, userPath, body: null);
        Assert.Equal(HttpStatusCode.NotFound, after.Status);
    }

    [Fact]
    public async Task JsonLedgerApi_reports_deleting_a_missing_user_as_404_USER_NOT_FOUND()
    {
        if (!EndpointDiscovery.IsLocalnetAvailable())
        {
            Assert.Skip(SkipMessage);
        }

        await using var fixture = LocalnetFixture.FromEnvironment();
        using var http = new HttpClient { BaseAddress = fixture.Endpoints.JsonLedgerApi };
        var token = await fixture.TokenProvider.GetAccessTokenAsync();
        var version = await ParticipantVersionAsync(http, token);

        var missing = await SendAsync(
            http,
            token,
            HttpMethod.Delete,
            $"v2/users/never-created-{fixture.PartyAllocator.InstanceSuffix}",
            body: null);

        Assert.True(
            missing.Status == HttpStatusCode.NotFound && JsonCode(missing.Body) == "USER_NOT_FOUND",
            "UserBuilder.DeleteAsync treats exactly '404 with code USER_NOT_FOUND' as a user that is already "
            + "gone when it retries a fixture teardown. If this fires after a Splice repin, that contract "
            + $"changed and UserBuilder must change with it. Participant version: {version}. "
            + $"Status: {(int)missing.Status}. Body: {missing.Body}");
    }

    private static string? JsonCode(string body)
    {
        using var document = JsonDocument.Parse(body);
        return document.RootElement.TryGetProperty("code", out var code) ? code.GetString() : null;
    }

    private static string RightsPayload(string userId, string party) =>
        JsonSerializer.Serialize(new Dictionary<string, object>
        {
            ["userId"] = userId,
            ["identityProviderId"] = string.Empty,
            ["rights"] = new[]
            {
                new Dictionary<string, object>
                {
                    ["kind"] = new Dictionary<string, object>
                    {
                        ["CanActAs"] = new { value = new { party } },
                    },
                },
            },
        });

    private static async Task<(HttpStatusCode Status, string Body)> SendAsync(
        HttpClient http,
        string token,
        HttpMethod method,
        string path,
        string? body)
    {
        using var request = new HttpRequestMessage(method, path);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        if (body is not null)
        {
            request.Content = new StringContent(body, Encoding.UTF8, "application/json");
        }
        using var response = await http.SendAsync(request);
        return (response.StatusCode, await response.Content.ReadAsStringAsync());
    }

    private static async Task<string> ParticipantVersionAsync(HttpClient http, string token)
    {
        var (status, body) = await SendAsync(http, token, HttpMethod.Get, "v2/version", body: null);
        return status == HttpStatusCode.OK ? body : $"<GET v2/version returned {(int)status}>";
    }

    private static int RightsCount(string body, string field)
    {
        using var document = JsonDocument.Parse(body);
        return document.RootElement.GetProperty(field).GetArrayLength();
    }

    private static void AssertPresentAndEmpty(string body, string field, string scenario, string version)
    {
        using var document = JsonDocument.Parse(body);
        Assert.True(
            document.RootElement.TryGetProperty(field, out var rights),
            $"The participant omitted '{field}' for {scenario} instead of reporting an empty array. "
            + "Peaceful.Canton.Localnet.Testing reads an absent field as 'the participant did not tell us': "
            + "UserBuilder.GrantRightsLeaseAsync treats it as fatal, and the lease's retry path cannot "
            + "confirm a hand-back without it. If this fires after a Splice repin, that contract changed "
            + $"and UserBuilder must change with it. Participant version: {version}. Body: {body}");
        Assert.Equal(JsonValueKind.Array, rights.ValueKind);
        Assert.Equal(0, rights.GetArrayLength());
    }
}
