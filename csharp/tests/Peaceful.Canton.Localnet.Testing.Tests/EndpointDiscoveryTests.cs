// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class EndpointDiscoveryTests
{
    [Theory]
    [InlineData(LocalnetProfile.BValidator1, "http://localhost:12975")]
    [InlineData(LocalnetProfile.AValidator1, "http://localhost:11975")]
    [InlineData(LocalnetProfile.CValidator1, "http://localhost:13975")]
    [InlineData(LocalnetProfile.SvValidator1, "http://localhost:10975")]
    [InlineData(LocalnetProfile.DValidator1, "http://localhost:14975")]
    public void Resolve_returns_default_json_api_url_per_profile(LocalnetProfile profile, string expected)
    {
        var env = SvSecretIfNeeded(profile);

        var endpoints = EndpointDiscovery.Resolve(profile, env);

        Assert.Equal(new Uri(expected), endpoints.JsonLedgerApi);
    }

    [Theory]
    [InlineData(LocalnetProfile.AValidator1, "a-validator-1-validator")]
    [InlineData(LocalnetProfile.BValidator1, "b-validator-1-validator")]
    [InlineData(LocalnetProfile.CValidator1, "c-validator-1-validator")]
    [InlineData(LocalnetProfile.DValidator1, "d-validator-1-validator")]
    public void Resolve_returns_default_client_id_per_profile(LocalnetProfile profile, string expected)
    {
        var endpoints = EndpointDiscovery.Resolve(profile, new Dictionary<string, string?>(StringComparer.Ordinal));

        Assert.Equal(expected, endpoints.ClientId);
        Assert.False(string.IsNullOrEmpty(endpoints.ClientSecret));
    }

    [Fact]
    public void Resolve_sv_validator_requires_explicit_client_secret()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);

        var ex = Assert.Throws<InvalidOperationException>(
            () => EndpointDiscovery.Resolve(LocalnetProfile.SvValidator1, env));
        Assert.Contains("CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET", ex.Message);
    }

    [Fact]
    public void Resolve_legacy_globals_apply_only_to_default_profile()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "a-validator-1",
            [EndpointDiscovery.JsonApiUrlEnv] = "http://legacy:99/",
            [EndpointDiscovery.ClientIdEnv] = "legacy-client",
            [EndpointDiscovery.ClientSecretEnv] = "legacy-secret",
        };

        var aEndpoints = EndpointDiscovery.Resolve(LocalnetProfile.AValidator1, env);
        var bEndpoints = EndpointDiscovery.Resolve(LocalnetProfile.BValidator1, env);

        Assert.Equal(new Uri("http://legacy:99/"), aEndpoints.JsonLedgerApi);
        Assert.Equal("legacy-client", aEndpoints.ClientId);
        Assert.Equal("legacy-secret", aEndpoints.ClientSecret);

        Assert.Equal(new Uri("http://localhost:12975"), bEndpoints.JsonLedgerApi);
        Assert.Equal("b-validator-1-validator", bEndpoints.ClientId);
        Assert.NotEqual("legacy-secret", bEndpoints.ClientSecret);
    }

    [Fact]
    public void ResolveForSlot_ignores_legacy_globals_for_default_profile()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "a-validator-1",
            [EndpointDiscovery.JsonApiUrlEnv] = "http://legacy:99/",
            [EndpointDiscovery.ClientIdEnv] = "legacy-client",
            [EndpointDiscovery.ClientSecretEnv] = "legacy-secret",
        };

        var endpoints = EndpointDiscovery.ResolveForSlot(LocalnetProfile.AValidator1, env);

        Assert.Equal(new Uri("http://localhost:11975"), endpoints.JsonLedgerApi);
        Assert.Equal("a-validator-1-validator", endpoints.ClientId);
    }

    [Fact]
    public void Resolve_per_slot_env_vars_override_defaults()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_B_VALIDATOR_1_JSON_API_URL"] = "https://b.example/json",
            ["CANTON_LOCALNET_B_VALIDATOR_1_TOKEN_URL"] = "https://b.example/token",
            ["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID"] = "b-custom",
            ["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET"] = "b-secret",
            ["CANTON_LOCALNET_B_VALIDATOR_1_AUDIENCE"] = "https://b.custom",
            ["CANTON_LOCALNET_B_VALIDATOR_1_SCOPE"] = "openid daml",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.BValidator1, env);

        Assert.Equal(new Uri("https://b.example/json"), endpoints.JsonLedgerApi);
        Assert.Equal(new Uri("https://b.example/token"), endpoints.TokenEndpoint);
        Assert.Equal("b-custom", endpoints.ClientId);
        Assert.Equal("b-secret", endpoints.ClientSecret);
        Assert.Equal("https://b.custom", endpoints.Audience);
        Assert.Equal("openid daml", endpoints.Scope);
    }

    [Fact]
    public void Resolve_per_slot_env_vars_win_over_legacy_globals()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "http://legacy:99",
            [EndpointDiscovery.ClientIdEnv] = "legacy",
            [EndpointDiscovery.ClientSecretEnv] = "legacy-secret",
            ["CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL"] = "http://per-slot:42",
            ["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"] = "per-slot",
            ["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"] = "per-slot-secret",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.AValidator1, env);

        Assert.Equal(new Uri("http://per-slot:42"), endpoints.JsonLedgerApi);
        Assert.Equal("per-slot", endpoints.ClientId);
        Assert.Equal("per-slot-secret", endpoints.ClientSecret);
    }

    [Fact]
    public void IsLocalnetAvailable_returns_false_when_credentials_missing()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:11975",
        };

        Assert.False(EndpointDiscovery.IsLocalnetAvailable(env));
    }

    [Fact]
    public void IsLocalnetAvailable_returns_true_when_legacy_globals_set()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:11975",
            [EndpointDiscovery.ClientIdEnv] = "client",
            [EndpointDiscovery.ClientSecretEnv] = "secret",
        };

        Assert.True(EndpointDiscovery.IsLocalnetAvailable(env));
    }

    [Fact]
    public void IsLocalnetAvailable_returns_true_when_per_slot_set_for_default_profile()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL"] = "http://localhost:11975",
            ["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID"] = "client",
            ["CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET"] = "secret",
        };

        Assert.True(EndpointDiscovery.IsLocalnetAvailable(env));
    }

    [Fact]
    public void IsSlotAvailable_returns_true_when_per_slot_trio_set_for_any_slot()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_B_VALIDATOR_1_JSON_API_URL"] = "http://localhost:12975",
            ["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID"] = "b-client",
            ["CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_SECRET"] = "b-secret",
        };

        Assert.True(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.BValidator1, env));
    }

    [Fact]
    public void IsSlotAvailable_returns_false_for_non_default_slot_when_only_legacy_globals_set()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "a-validator-1",
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:11975",
            [EndpointDiscovery.ClientIdEnv] = "legacy-client",
            [EndpointDiscovery.ClientSecretEnv] = "legacy-secret",
        };

        Assert.True(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.AValidator1, env));
        Assert.False(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.BValidator1, env));
    }

    [Fact]
    public void IsSlotAvailable_returns_false_when_neither_per_slot_nor_legacy_credentials_set()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);

        Assert.False(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.AValidator1, env));
        Assert.False(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.BValidator1, env));
    }

    [Theory]
    [InlineData("b-validator-1", LocalnetProfile.BValidator1)]
    [InlineData("B-VALIDATOR-1", LocalnetProfile.BValidator1)]
    [InlineData("a-validator-1", LocalnetProfile.AValidator1)]
    [InlineData("c-validator-1", LocalnetProfile.CValidator1)]
    [InlineData("sv-validator-1", LocalnetProfile.SvValidator1)]
    [InlineData("super-validator", LocalnetProfile.SvValidator1)]
    [InlineData("d-validator-1", LocalnetProfile.DValidator1)]
    [InlineData("D-VALIDATOR-1", LocalnetProfile.DValidator1)]
    public void ResolveProfile_maps_string_to_enum(string raw, LocalnetProfile expected)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = raw,
        };

        Assert.Equal(expected, EndpointDiscovery.ResolveProfile(env));
    }

    [Fact]
    public void ResolveProfile_defaults_to_a_validator_1()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);
        Assert.Equal(LocalnetProfile.AValidator1, EndpointDiscovery.ResolveProfile(env));
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

    private static Dictionary<string, string?> SvSecretIfNeeded(LocalnetProfile profile)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);
        if (profile == LocalnetProfile.SvValidator1)
        {
            env["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-test-secret";
        }
        return env;
    }
}
