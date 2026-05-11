// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Selects which Canton LocalNet profile a fixture is wired to. The profile
/// determines the host-exposed JSON Ledger API port (app-user=2xxx,
/// app-provider=3xxx, sv=4xxx) and which Keycloak realm issues tokens.
/// </summary>
public enum LocalnetProfile
{
    AppUser,
    AppProvider,
    Sv,
}

/// <summary>
/// Holds the resolved endpoints a fixture needs: the JSON Ledger API base URL,
/// the OAuth2 token endpoint, the expected token audience, and the
/// client_credentials client id/secret.
/// </summary>
public sealed record LocalnetEndpoints(
    Uri JsonLedgerApi,
    Uri TokenEndpoint,
    string Audience,
    string ClientId,
    string ClientSecret,
    string? Scope);

/// <summary>
/// Resolves the URLs and credentials a <see cref="LocalnetFixture"/> needs from
/// environment variables. Defaults match the compose stack in
/// <c>compose/modules/localnet/env/common.env</c> and the Keycloak realm
/// configurations in <c>compose/modules/keycloak/env/</c>.
/// </summary>
public static class EndpointDiscovery
{
    public const string JsonApiUrlEnv = "CANTON_LOCALNET_JSON_API_URL";
    public const string TokenUrlEnv = "CANTON_LOCALNET_TOKEN_URL";
    public const string AudienceEnv = "CANTON_LOCALNET_AUDIENCE";
    public const string ClientIdEnv = "CANTON_LOCALNET_CLIENT_ID";
    public const string ClientSecretEnv = "CANTON_LOCALNET_CLIENT_SECRET";
    public const string ScopeEnv = "CANTON_LOCALNET_SCOPE";
    public const string ProfileEnv = "CANTON_LOCALNET_PROFILE";

    private const string DefaultAudience = "https://canton.network.global";
    private const string DefaultScope = "openid";
    private const string DefaultKeycloakHost = "http://localhost:8082";

    /// <summary>
    /// Returns true when enough environment variables are set to drive a real
    /// fixture against a running LocalNet. Used to gate integration tests so
    /// they skip cleanly on a developer machine without docker compose up.
    /// </summary>
    public static bool IsLocalnetAvailable(IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        return !string.IsNullOrEmpty(GetValue(env, JsonApiUrlEnv))
            && !string.IsNullOrEmpty(GetValue(env, ClientIdEnv))
            && !string.IsNullOrEmpty(GetValue(env, ClientSecretEnv));
    }

    /// <summary>
    /// Resolves endpoints for the given profile. Explicit env vars override the
    /// profile-derived defaults; <see cref="ClientIdEnv"/> and
    /// <see cref="ClientSecretEnv"/> are required and have no fallback.
    /// </summary>
    public static LocalnetEndpoints Resolve(
        LocalnetProfile profile = LocalnetProfile.AppProvider,
        IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        var jsonApi = GetValue(env, JsonApiUrlEnv) ?? DefaultJsonApiUrl(profile);
        var tokenUrl = GetValue(env, TokenUrlEnv) ?? DefaultTokenUrl(profile);
        var audience = GetValue(env, AudienceEnv) ?? DefaultAudience;
        var clientId = GetValue(env, ClientIdEnv)
            ?? throw new InvalidOperationException(
                $"Environment variable '{ClientIdEnv}' is required to talk to the LocalNet token endpoint.");
        var clientSecret = GetValue(env, ClientSecretEnv)
            ?? throw new InvalidOperationException(
                $"Environment variable '{ClientSecretEnv}' is required to talk to the LocalNet token endpoint.");
        var scope = GetValue(env, ScopeEnv) ?? DefaultScope;

        return new LocalnetEndpoints(
            JsonLedgerApi: new Uri(jsonApi, UriKind.Absolute),
            TokenEndpoint: new Uri(tokenUrl, UriKind.Absolute),
            Audience: audience,
            ClientId: clientId,
            ClientSecret: clientSecret,
            Scope: scope);
    }

    /// <summary>
    /// Returns the profile selected by <see cref="ProfileEnv"/>, defaulting to
    /// <see cref="LocalnetProfile.AppProvider"/>.
    /// </summary>
    public static LocalnetProfile ResolveProfile(IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        var raw = GetValue(env, ProfileEnv);
        if (string.IsNullOrWhiteSpace(raw))
        {
            return LocalnetProfile.AppProvider;
        }

        return raw.Trim().ToLowerInvariant() switch
        {
            "app-user" or "appuser" => LocalnetProfile.AppUser,
            "app-provider" or "appprovider" => LocalnetProfile.AppProvider,
            "sv" or "super-validator" or "supervalidator" => LocalnetProfile.Sv,
            _ => throw new InvalidOperationException(
                $"Unknown profile '{raw}' in {ProfileEnv}; expected one of: app-user, app-provider, sv."),
        };
    }

    private static string DefaultJsonApiUrl(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.AppUser => "http://localhost:2975",
        LocalnetProfile.AppProvider => "http://localhost:3975",
        LocalnetProfile.Sv => "http://localhost:4975",
        _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
    };

    private static string DefaultTokenUrl(LocalnetProfile profile)
    {
        var realm = profile switch
        {
            LocalnetProfile.AppUser => "AppUser",
            LocalnetProfile.AppProvider => "AppProvider",
            LocalnetProfile.Sv => "AppProvider",
            _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
        };
        return $"{DefaultKeycloakHost}/realms/{realm}/protocol/openid-connect/token";
    }

    private static string? GetValue(IReadOnlyDictionary<string, string?> env, string key)
    {
        if (!env.TryGetValue(key, out var value))
        {
            return null;
        }
        return string.IsNullOrEmpty(value) ? null : value;
    }

    private static IReadOnlyDictionary<string, string?> Snapshot()
    {
        var dict = new Dictionary<string, string?>(StringComparer.Ordinal);
        foreach (System.Collections.DictionaryEntry entry in Environment.GetEnvironmentVariables())
        {
            if (entry.Key is string key)
            {
                dict[key] = entry.Value as string;
            }
        }
        return dict;
    }
}
