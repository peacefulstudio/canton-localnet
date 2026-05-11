// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class EndpointDiscoveryTests
{
    [Theory]
    [InlineData(LocalnetProfile.AppUser, "http://localhost:2975")]
    [InlineData(LocalnetProfile.AppProvider, "http://localhost:3975")]
    [InlineData(LocalnetProfile.Sv, "http://localhost:4975")]
    public void Resolve_returns_default_json_api_url_per_profile(LocalnetProfile profile, string expected)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ClientIdEnv] = "client",
            [EndpointDiscovery.ClientSecretEnv] = "secret",
        };

        var endpoints = EndpointDiscovery.Resolve(profile, env);

        Assert.Equal(new Uri(expected), endpoints.JsonLedgerApi);
    }

    [Fact]
    public void Resolve_throws_when_client_id_missing()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ClientSecretEnv] = "secret",
        };

        Assert.Throws<InvalidOperationException>(() => EndpointDiscovery.Resolve(environment: env));
    }

    [Fact]
    public void Resolve_overrides_defaults_with_explicit_env_vars()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "https://example.com/json",
            [EndpointDiscovery.TokenUrlEnv] = "https://example.com/token",
            [EndpointDiscovery.AudienceEnv] = "https://custom",
            [EndpointDiscovery.ClientIdEnv] = "my-client",
            [EndpointDiscovery.ClientSecretEnv] = "my-secret",
            [EndpointDiscovery.ScopeEnv] = "daml_ledger_api",
        };

        var endpoints = EndpointDiscovery.Resolve(environment: env);

        Assert.Equal(new Uri("https://example.com/json"), endpoints.JsonLedgerApi);
        Assert.Equal(new Uri("https://example.com/token"), endpoints.TokenEndpoint);
        Assert.Equal("https://custom", endpoints.Audience);
        Assert.Equal("my-client", endpoints.ClientId);
        Assert.Equal("my-secret", endpoints.ClientSecret);
        Assert.Equal("daml_ledger_api", endpoints.Scope);
    }

    [Fact]
    public void IsLocalnetAvailable_returns_false_when_credentials_missing()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:3975",
        };

        Assert.False(EndpointDiscovery.IsLocalnetAvailable(env));
    }

    [Fact]
    public void IsLocalnetAvailable_returns_true_when_all_required_set()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:3975",
            [EndpointDiscovery.ClientIdEnv] = "client",
            [EndpointDiscovery.ClientSecretEnv] = "secret",
        };

        Assert.True(EndpointDiscovery.IsLocalnetAvailable(env));
    }

    [Theory]
    [InlineData("app-user", LocalnetProfile.AppUser)]
    [InlineData("APP-USER", LocalnetProfile.AppUser)]
    [InlineData("app-provider", LocalnetProfile.AppProvider)]
    [InlineData("sv", LocalnetProfile.Sv)]
    [InlineData("super-validator", LocalnetProfile.Sv)]
    public void ResolveProfile_maps_string_to_enum(string raw, LocalnetProfile expected)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = raw,
        };

        Assert.Equal(expected, EndpointDiscovery.ResolveProfile(env));
    }

    [Fact]
    public void ResolveProfile_defaults_to_app_provider()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);
        Assert.Equal(LocalnetProfile.AppProvider, EndpointDiscovery.ResolveProfile(env));
    }

    [Fact]
    public void ResolveProfile_throws_on_unknown_value()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "bogus",
        };
        Assert.Throws<InvalidOperationException>(() => EndpointDiscovery.ResolveProfile(env));
    }
}
