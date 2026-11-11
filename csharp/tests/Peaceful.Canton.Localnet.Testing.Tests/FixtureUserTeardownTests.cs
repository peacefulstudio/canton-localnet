// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

[Collection("EnvVarMutation")]
public class FixtureUserTeardownTests : IDisposable
{
    private const string UserNotFound =
        """{"code":"USER_NOT_FOUND","cause":"deleting user failed for unknown user \"gone\"","errorCategory":11}""";

    private const string OtherNotFound = """{"code":"PACKAGE_NOT_FOUND","cause":"not a user"}""";

    private static readonly string[] SlotKeys =
    [
        "CANTON_LOCALNET_B_VALIDATOR_1_JSON_API_URL",
        "CANTON_LOCALNET_B_VALIDATOR_1_TOKEN_URL",
        "CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID",
        "CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET",
    ];

    private readonly Dictionary<string, string?> _savedSlotEnv =
        SlotKeys.ToDictionary(key => key, Environment.GetEnvironmentVariable);

    public void Dispose()
    {
        foreach (var (key, value) in _savedSlotEnv)
        {
            Environment.SetEnvironmentVariable(key, value);
        }
    }

    private static string Path(string userId) => $"/v2/users/{userId}";

    private static void PointBValidatorAt(ScriptedLedger ledger)
    {
        Environment.SetEnvironmentVariable(SlotKeys[0], ledger.Root.ToString());
        Environment.SetEnvironmentVariable(SlotKeys[1], ledger.Endpoints.TokenEndpoint.ToString());
        Environment.SetEnvironmentVariable(SlotKeys[2], "id");
        Environment.SetEnvironmentVariable(SlotKeys[3], "sec");
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_each_user_created_through_CreateUserAsync()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await fixture.CreateUserAsync("bob-user");

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user", "bob-user" }, ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_issues_no_request_when_no_user_was_created()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);

        await fixture.DisposeAsync();

        Assert.Empty(ledger.Hits);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_leaves_users_created_through_UserBuilder_directly()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.UserBuilder.CreateAsync("direct-user");

        await fixture.DisposeAsync();

        Assert.Empty(ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_issues_no_get_request()
    {
        using var ledger = ScriptedLedger.Start((_, _) => new LedgerReply(503, "{}"));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        Assert.DoesNotContain(ledger.Hits, hit => hit.Method == "GET");
        Assert.Equal(2, ledger.DeletedUserIds.Count);
    }

    [Theory]
    [InlineData(404, UserNotFound)]
    [InlineData(404, OtherNotFound)]
    [InlineData(500, "{}")]
    [InlineData(503, "")]
    public async Task LocalnetFixture_dispose_throws_when_the_first_delete_attempt_fails(int status, string body)
    {
        using var ledger = ScriptedLedger.Start((_, _) => new LedgerReply(status, body));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        Assert.Equal((System.Net.HttpStatusCode)status, exception.StatusCode);
        Assert.Contains("alice-user", exception.Message);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_retry_treats_a_missing_user_as_success()
    {
        using var ledger = ScriptedLedger.Start((_, call) => call == 1
            ? new LedgerReply(503, "{}")
            : new LedgerReply(404, UserNotFound));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user", "alice-user" }, ledger.DeletedUserIds);
    }

    [Theory]
    [InlineData(404, OtherNotFound)]
    [InlineData(404, "")]
    [InlineData(500, UserNotFound)]
    [InlineData(503, "{}")]
    public async Task LocalnetFixture_dispose_retry_still_throws_when_the_response_is_not_a_missing_user(int status, string body)
    {
        using var ledger = ScriptedLedger.Start((_, call) => call == 1
            ? new LedgerReply(503, "{}")
            : new LedgerReply(status, body));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        Assert.Equal((System.Net.HttpStatusCode)status, exception.StatusCode);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_retry_deletes_only_the_users_still_outstanding()
    {
        using var ledger = ScriptedLedger.Start((user, call) => user == "bob-user" && call == 1
            ? new LedgerReply(503, "{}")
            : new LedgerReply(200, "{}"));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await fixture.CreateUserAsync("bob-user");
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user", "bob-user", "bob-user" }, ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_counts_attempts_per_user_for_a_user_created_after_a_failed_dispose()
    {
        using var ledger = ScriptedLedger.Start((user, call) => user == "alice-user"
            ? call == 1 ? new LedgerReply(503, "{}") : new LedgerReply(200, "{}")
            : new LedgerReply(404, UserNotFound));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());
        await fixture.CreateUserAsync("bob-user");

        var exception = await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        Assert.Equal(System.Net.HttpStatusCode.NotFound, exception.StatusCode);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_the_remaining_users_when_one_delete_fails()
    {
        using var ledger = ScriptedLedger.Start((user, _) => user == "alice-user"
            ? new LedgerReply(503, "{}")
            : new LedgerReply(200, "{}"));
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await fixture.CreateUserAsync("bob-user");

        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        Assert.Equal(new[] { "alice-user", "bob-user" }, ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_issues_no_further_request_once_every_user_is_deleted()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await fixture.DisposeAsync();

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user" }, ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_the_user_after_its_rights_lease_is_handed_back()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        var lease = await fixture.GrantUserRightsLeaseAsync("alice-user", actAs: new[] { "alice::p" });
        await lease.DisposeAsync();

        await fixture.DisposeAsync();

        Assert.Equal(
            new[]
            {
                new LedgerHit("POST", "/v2/users"),
                new LedgerHit("POST", Path("alice-user") + "/rights"),
                new LedgerHit("PATCH", Path("alice-user") + "/rights"),
                new LedgerHit("DELETE", Path("alice-user")),
            },
            ledger.Hits);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_users_created_through_the_default_slot_view_once()
    {
        using var ledger = ScriptedLedger.Start();
        var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);
        await fixture.Validator("a-validator-1").CreateUserAsync("alice-user");

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user" }, ledger.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_deletes_users_created_through_another_slot_view_on_that_slot()
    {
        using var defaultSlot = ScriptedLedger.Start();
        using var slotB = ScriptedLedger.Start();
        PointBValidatorAt(slotB);
        var fixture = LocalnetFixture.FromEndpoints(defaultSlot.Endpoints);
        await fixture.CreateUserAsync("alice-user");
        await fixture.Validator("b-validator-1").CreateUserAsync("bob-user");

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "alice-user" }, defaultSlot.DeletedUserIds);
        Assert.Equal(new[] { "bob-user" }, slotB.DeletedUserIds);
    }

    [Fact]
    public async Task LocalnetFixture_dispose_retry_reaches_users_on_another_slot_after_a_failed_first_attempt()
    {
        using var defaultSlot = ScriptedLedger.Start();
        using var slotB = ScriptedLedger.Start((_, call) => call == 1
            ? new LedgerReply(503, "{}")
            : new LedgerReply(404, UserNotFound));
        PointBValidatorAt(slotB);
        var fixture = LocalnetFixture.FromEndpoints(defaultSlot.Endpoints);
        await fixture.Validator("b-validator-1").CreateUserAsync("bob-user");
        await Assert.ThrowsAsync<JsonLedgerApiException>(async () => await fixture.DisposeAsync());

        await fixture.DisposeAsync();

        Assert.Equal(new[] { "bob-user", "bob-user" }, slotB.DeletedUserIds);
    }
}
