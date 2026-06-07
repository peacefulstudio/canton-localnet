// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class EndpointDiscoveryTests
{
    [Theory]
    [InlineData(LocalnetProfile.SvValidator1, "http://localhost:10975")]
    [InlineData(LocalnetProfile.AValidator1, "http://localhost:11975")]
    [InlineData(LocalnetProfile.BValidator1, "http://localhost:12975")]
    [InlineData(LocalnetProfile.CValidator1, "http://localhost:13975")]
    [InlineData(LocalnetProfile.DValidator1, "http://localhost:14975")]
    public void Resolve_returns_default_json_api_url_per_profile(LocalnetProfile profile, string expected)
    {
        var env = SvCredentialsIfNeeded(profile);

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

    [Theory]
    [InlineData(LocalnetProfile.AValidator1, "http://localhost:8082/realms/AValidator1/protocol/openid-connect/token")]
    [InlineData(LocalnetProfile.BValidator1, "http://localhost:8082/realms/BValidator1/protocol/openid-connect/token")]
    [InlineData(LocalnetProfile.CValidator1, "http://localhost:8082/realms/CValidator1/protocol/openid-connect/token")]
    [InlineData(LocalnetProfile.DValidator1, "http://localhost:8082/realms/DValidator1/protocol/openid-connect/token")]
    public void Resolve_returns_default_token_url_per_profile(LocalnetProfile profile, string expected)
    {
        var endpoints = EndpointDiscovery.Resolve(profile, new Dictionary<string, string?>(StringComparer.Ordinal));

        Assert.Equal(new Uri(expected), endpoints.TokenEndpoint);
    }

    [Fact]
    public void Resolve_defaults_validator_user_id_for_a_validator_1()
    {
        var endpoints = EndpointDiscovery.Resolve(
            LocalnetProfile.AValidator1,
            new Dictionary<string, string?>(StringComparer.Ordinal));

        Assert.Equal("c87743ab-80e0-4b83-935a-4c0582226691", endpoints.ValidatorUserId);
    }

    [Fact]
    public void Resolve_namespaced_env_overrides_validator_user_id()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID"] = "custom-user-id",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.AValidator1, env);

        Assert.Equal("custom-user-id", endpoints.ValidatorUserId);
    }

    [Fact]
    public void Resolve_legacy_validator_user_id_env_applies_to_default_profile()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "a-validator-1",
            [EndpointDiscovery.ValidatorUserIdEnv] = "legacy-user-id",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.AValidator1, env);

        Assert.Equal("legacy-user-id", endpoints.ValidatorUserId);
    }

    [Fact]
    public void Resolve_namespaced_validator_user_id_wins_over_legacy_global()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ValidatorUserIdEnv] = "legacy-user-id",
            ["CANTON_LOCALNET_A_VALIDATOR_1_VALIDATOR_USER_ID"] = "per-slot-user-id",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.AValidator1, env);

        Assert.Equal("per-slot-user-id", endpoints.ValidatorUserId);
    }

    [Fact]
    public void Resolve_legacy_validator_user_id_env_does_not_apply_to_non_default_slot()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "a-validator-1",
            [EndpointDiscovery.ValidatorUserIdEnv] = "legacy-user-id",
        };

        var bEndpoints = EndpointDiscovery.Resolve(LocalnetProfile.BValidator1, env);

        Assert.Equal(string.Empty, bEndpoints.ValidatorUserId);
    }

    [Fact]
    public void Resolve_sv_validator_has_no_oauth2_defaults()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);

        var ex = Assert.Throws<InvalidOperationException>(
            () => EndpointDiscovery.Resolve(LocalnetProfile.SvValidator1, env));
        Assert.Contains("TOKEN_URL", ex.Message);
    }

    [Theory]
    [InlineData("CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL")]
    [InlineData("CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID")]
    [InlineData("CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET")]
    public void Resolve_sv_validator_requires_each_oauth2_field(string missingKey)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL"] = "https://sv.example/token",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID"] = "sv-client",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-secret",
        };
        env.Remove(missingKey);

        var ex = Assert.Throws<InvalidOperationException>(
            () => EndpointDiscovery.Resolve(LocalnetProfile.SvValidator1, env));
        Assert.Contains(missingKey, ex.Message);
    }

    [Fact]
    public void Resolve_sv_validator_succeeds_with_explicit_oauth2_config()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL"] = "https://sv.example/token",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID"] = "sv-client",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-secret",
        };

        var endpoints = EndpointDiscovery.Resolve(LocalnetProfile.SvValidator1, env);

        Assert.Equal(new Uri("https://sv.example/token"), endpoints.TokenEndpoint);
        Assert.Equal("sv-client", endpoints.ClientId);
        Assert.Equal("sv-secret", endpoints.ClientSecret);
        Assert.Equal(new Uri("http://localhost:10975"), endpoints.JsonLedgerApi);
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

    [Fact]
    public void IsSlotAvailable_returns_false_for_sv_when_per_slot_trio_set_but_token_url_missing()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_SV_VALIDATOR_1_JSON_API_URL"] = "http://localhost:10975",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID"] = "sv-client",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-secret",
        };

        Assert.False(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.SvValidator1, env));
    }

    [Fact]
    public void IsSlotAvailable_returns_true_for_sv_when_per_slot_token_url_also_set()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            ["CANTON_LOCALNET_SV_VALIDATOR_1_JSON_API_URL"] = "http://localhost:10975",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL"] = "https://sv.example/token",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID"] = "sv-client",
            ["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-secret",
        };

        Assert.True(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.SvValidator1, env));
    }

    [Fact]
    public void IsSlotAvailable_returns_false_for_default_sv_when_legacy_token_url_missing()
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal)
        {
            [EndpointDiscovery.ProfileEnv] = "sv-validator-1",
            [EndpointDiscovery.JsonApiUrlEnv] = "http://localhost:10975",
            [EndpointDiscovery.ClientIdEnv] = "sv-client",
            [EndpointDiscovery.ClientSecretEnv] = "sv-secret",
        };

        Assert.False(EndpointDiscovery.IsSlotAvailable(LocalnetProfile.SvValidator1, env));
    }

    [Theory]
    [InlineData("sv-validator-1", LocalnetProfile.SvValidator1)]
    [InlineData("super-validator", LocalnetProfile.SvValidator1)]
    [InlineData("a-validator-1", LocalnetProfile.AValidator1)]
    [InlineData("b-validator-1", LocalnetProfile.BValidator1)]
    [InlineData("B-VALIDATOR-1", LocalnetProfile.BValidator1)]
    [InlineData("c-validator-1", LocalnetProfile.CValidator1)]
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

    private static Dictionary<string, string?> SvCredentialsIfNeeded(LocalnetProfile profile)
    {
        var env = new Dictionary<string, string?>(StringComparer.Ordinal);
        if (profile == LocalnetProfile.SvValidator1)
        {
            env["CANTON_LOCALNET_SV_VALIDATOR_1_TOKEN_URL"] = "https://sv.example/token";
            env["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_ID"] = "sv-test-client";
            env["CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET"] = "sv-test-secret";
        }
        return env;
    }
}
