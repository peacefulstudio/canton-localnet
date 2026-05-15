// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

[Collection("EnvVarMutation")]
public class ValidatorFixtureTests : IDisposable
{
    private const string FakeClientId = "test-client";
    private const string FakeClientSecret = "test-secret";

    private const string SvSecretEnv = "CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET";

    private readonly string? _savedClientId;
    private readonly string? _savedClientSecret;
    private readonly string? _savedSvSecret;

    public ValidatorFixtureTests()
    {
        _savedClientId = Environment.GetEnvironmentVariable(EndpointDiscovery.ClientIdEnv);
        _savedClientSecret = Environment.GetEnvironmentVariable(EndpointDiscovery.ClientSecretEnv);
        _savedSvSecret = Environment.GetEnvironmentVariable(SvSecretEnv);
        Environment.SetEnvironmentVariable(EndpointDiscovery.ClientIdEnv, FakeClientId);
        Environment.SetEnvironmentVariable(EndpointDiscovery.ClientSecretEnv, FakeClientSecret);
        Environment.SetEnvironmentVariable(SvSecretEnv, "sv-test-secret");
    }

    public void Dispose()
    {
        Environment.SetEnvironmentVariable(EndpointDiscovery.ClientIdEnv, _savedClientId);
        Environment.SetEnvironmentVariable(EndpointDiscovery.ClientSecretEnv, _savedClientSecret);
        Environment.SetEnvironmentVariable(SvSecretEnv, _savedSvSecret);
    }

    private static LocalnetFixture NewFixture(LocalnetProfile profile = LocalnetProfile.AValidator1)
    {
        var endpoints = EndpointDiscovery.Resolve(profile);
        return LocalnetFixture.FromEndpoints(endpoints, profile);
    }

    [Theory]
    [InlineData("sv-validator-1", LocalnetProfile.SvValidator1)]
    [InlineData("a-validator-1", LocalnetProfile.AValidator1)]
    [InlineData("b-validator-1", LocalnetProfile.BValidator1)]
    [InlineData("c-validator-1", LocalnetProfile.CValidator1)]
    [InlineData("d-validator-1", LocalnetProfile.DValidator1)]
    public async Task Validator_by_slot_name_returns_view_for_matching_profile(string slot, LocalnetProfile expected)
    {
        await using var fixture = NewFixture();

        var view = fixture.Validator(slot);

        Assert.Equal(expected, view.Profile);
        Assert.Equal(slot, view.Slot);
    }

    [Fact]
    public async Task Validator_for_default_slot_shares_clients_with_root_fixture()
    {
        await using var fixture = NewFixture(LocalnetProfile.AValidator1);

        var view = fixture.Validator("a-validator-1");

        Assert.Same(fixture.AdminClient, view.AdminClient);
        Assert.Same(fixture.PartyAllocator, view.PartyAllocator);
        Assert.Same(fixture.DarUploader, view.DarUploader);
        Assert.Same(fixture.UserBuilder, view.UserBuilder);
    }

    [Fact]
    public async Task Validator_for_other_slot_builds_scoped_clients()
    {
        await using var fixture = NewFixture(LocalnetProfile.AValidator1);

        var a = fixture.Validator("a-validator-1");
        var b = fixture.Validator("b-validator-1");

        Assert.NotSame(a.AdminClient, b.AdminClient);
        Assert.NotEqual(a.Endpoints.JsonLedgerApi, b.Endpoints.JsonLedgerApi);
        Assert.Equal(new Uri("http://localhost:12975"), b.Endpoints.JsonLedgerApi);
    }

    [Fact]
    public async Task Validator_for_non_default_slot_does_not_inherit_legacy_global_env_vars()
    {
        var savedJsonApi = Environment.GetEnvironmentVariable(EndpointDiscovery.JsonApiUrlEnv);
        Environment.SetEnvironmentVariable(EndpointDiscovery.JsonApiUrlEnv, "http://legacy-a:11975");
        try
        {
            await using var fixture = NewFixture(LocalnetProfile.AValidator1);

            var a = fixture.Validator("a-validator-1");
            var b = fixture.Validator("b-validator-1");

            Assert.NotEqual(a.Endpoints.JsonLedgerApi, b.Endpoints.JsonLedgerApi);
            Assert.NotEqual(a.Endpoints.ClientId, b.Endpoints.ClientId);
            Assert.Equal(new Uri("http://localhost:12975"), b.Endpoints.JsonLedgerApi);
            Assert.Equal("b-validator-1-validator", b.Endpoints.ClientId);
        }
        finally
        {
            Environment.SetEnvironmentVariable(EndpointDiscovery.JsonApiUrlEnv, savedJsonApi);
        }
    }

    [Fact]
    public async Task Validator_returns_cached_instance_on_repeated_calls()
    {
        await using var fixture = NewFixture(LocalnetProfile.AValidator1);

        var first = fixture.Validator("c-validator-1");
        var second = fixture.Validator("c-validator-1");

        Assert.Same(first, second);
    }

    [Fact]
    public async Task Validator_unknown_slot_throws_argument_exception()
    {
        await using var fixture = NewFixture();

        var exception = Assert.Throws<ArgumentException>(() => fixture.Validator("e-validator-1"));
        Assert.Contains("Unknown slot", exception.Message);
    }

    [Fact]
    public async Task Validator_blank_slot_throws_argument_exception()
    {
        await using var fixture = NewFixture();
        Assert.Throws<ArgumentException>(() => fixture.Validator(""));
        Assert.Throws<ArgumentException>(() => fixture.Validator("   "));
    }

    [Fact]
    public void KnownSlots_returns_five_canonical_slots_in_stable_order()
    {
        var slots = LocalnetFixture.KnownSlots();
        Assert.Equal(new[]
        {
            "sv-validator-1",
            "a-validator-1",
            "b-validator-1",
            "c-validator-1",
            "d-validator-1",
        }, slots);
    }
}
