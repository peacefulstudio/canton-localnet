// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

namespace Peaceful.Canton.Localnet.Testing;

/// <summary>
/// Selects which Canton LocalNet profile a fixture is wired to. The profile
/// determines the host-exposed JSON Ledger API port (sv-validator-1=10xxx,
/// a-validator-1=11xxx, b-validator-1=12xxx, c-validator-1=13xxx,
/// d-validator-1=14xxx) and which
/// Keycloak realm issues tokens.
/// </summary>
public enum LocalnetProfile
{
    BValidator1,
    AValidator1,
    CValidator1,
    SvValidator1,
    DValidator1,
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
///
/// <para>
/// <b>Discovery shape (see ADR-0003):</b> every override variable is
/// namespaced with the canonical slot in <c>SCREAMING_SNAKE_CASE</c>, e.g.
/// <c>CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL</c>,
/// <c>CANTON_LOCALNET_B_VALIDATOR_1_CLIENT_ID</c>. Defaults derive from the
/// slot's two-digit port prefix (ADR-0002) and the slot's Keycloak realm.
/// The legacy un-namespaced globals
/// (<see cref="JsonApiUrlEnv"/>, <see cref="ClientIdEnv"/>,
/// <see cref="ClientSecretEnv"/>, <see cref="TokenUrlEnv"/>,
/// <see cref="AudienceEnv"/>, <see cref="ScopeEnv"/>) are honoured for
/// the selected fixture profile only — they never leak into other slots'
/// resolution paths, so <see cref="LocalnetFixture.Validator(string)"/>
/// always routes to the slot's own endpoints. Per-slot variables win
/// over the matching legacy global.
/// </para>
///
/// <para>
/// SECURITY: the per-slot <c>CLIENT_SECRET</c> defaults for a, b, c, d are
/// the public LocalNet demo credentials shipped with the splice quickstart.
/// They are valid only against an ephemeral local Keycloak realm — never
/// use them in any non-localhost or production deployment. The
/// sv-validator-1 secret has no default and must be set explicitly.
/// </para>
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

    private const string AValidator1DemoClientSecret = "AL8648b9SfdTFImq7FV56Vd0KHifHBuC";
    private const string BcdValidator1DemoClientSecret = "6m12QyyGl81d9nABWQXMycZdXho6ejEX";

    /// <summary>
    /// Returns true when enough environment variables are set to drive a real
    /// fixture against a running LocalNet. Used to gate integration tests so
    /// they skip cleanly on a developer machine without docker compose up.
    /// Considers either the legacy global vars or a per-slot
    /// <c>CANTON_LOCALNET_&lt;SLOT&gt;_JSON_API_URL</c> + matching
    /// <c>CLIENT_ID</c>/<c>CLIENT_SECRET</c> for the resolved profile to
    /// satisfy availability.
    /// </summary>
    public static bool IsLocalnetAvailable(IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        return IsSlotAvailable(ResolveProfile(env), env);
    }

    /// <summary>
    /// Returns true when enough environment variables are set to drive a
    /// fixture view of the given slot against a running LocalNet. Mirrors the
    /// precedence rules of <see cref="Resolve(LocalnetProfile, IReadOnlyDictionary{string, string?})"/>:
    /// the per-slot trio (<c>CANTON_LOCALNET_&lt;SLOT&gt;_JSON_API_URL</c>,
    /// <c>CLIENT_ID</c>, <c>CLIENT_SECRET</c>) satisfies availability for any
    /// slot; the legacy un-namespaced globals only satisfy availability for
    /// the fixture's resolved default profile, so a multi-slot integration
    /// test can gate the non-default side independently and skip cleanly when
    /// only the default slot is reachable.
    /// </summary>
    public static bool IsSlotAvailable(
        LocalnetProfile profile,
        IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        var slot = SlotEnvPrefix(profile);
        if (!string.IsNullOrEmpty(GetValue(env, $"CANTON_LOCALNET_{slot}_JSON_API_URL"))
            && !string.IsNullOrEmpty(GetValue(env, $"CANTON_LOCALNET_{slot}_CLIENT_ID"))
            && !string.IsNullOrEmpty(GetValue(env, $"CANTON_LOCALNET_{slot}_CLIENT_SECRET")))
        {
            return true;
        }
        if (profile != ResolveProfile(env))
        {
            return false;
        }
        return !string.IsNullOrEmpty(GetValue(env, JsonApiUrlEnv))
            && !string.IsNullOrEmpty(GetValue(env, ClientIdEnv))
            && !string.IsNullOrEmpty(GetValue(env, ClientSecretEnv));
    }

    /// <summary>
    /// Resolves endpoints for the given profile. Per-slot env vars
    /// (<c>CANTON_LOCALNET_&lt;SLOT&gt;_*</c>) take precedence over the
    /// legacy un-namespaced globals; the legacy globals only apply when
    /// <paramref name="profile"/> is the fixture's selected default, so
    /// each per-slot view resolves its own credentials independently.
    /// Both credential paths can fall back to the public LocalNet demo
    /// secrets shipped with the compose stack; the sv-validator-1 secret
    /// has no default and must be set via env (<see cref="ClientSecretEnv"/>
    /// for the default-profile case, or
    /// <c>CANTON_LOCALNET_SV_VALIDATOR_1_CLIENT_SECRET</c> for the per-slot
    /// case).
    /// </summary>
    public static LocalnetEndpoints Resolve(
        LocalnetProfile profile = LocalnetProfile.AValidator1,
        IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        var defaultProfile = ResolveProfile(env);
        var honourLegacyGlobals = profile == defaultProfile;
        return Resolve(profile, env, honourLegacyGlobals);
    }

    /// <summary>
    /// Resolves endpoints for the given slot ignoring the legacy
    /// un-namespaced globals (<see cref="JsonApiUrlEnv"/>,
    /// <see cref="ClientIdEnv"/>, etc.). Use this when wiring a per-slot view
    /// from <see cref="LocalnetFixture.Validator(LocalnetProfile)"/> so that
    /// a global env var meant for one slot cannot leak into another.
    /// </summary>
    public static LocalnetEndpoints ResolveForSlot(
        LocalnetProfile profile,
        IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        return Resolve(profile, env, honourLegacyGlobals: false);
    }

    private static LocalnetEndpoints Resolve(
        LocalnetProfile profile,
        IReadOnlyDictionary<string, string?> env,
        bool honourLegacyGlobals)
    {
        var slot = SlotEnvPrefix(profile);

        var jsonApi = GetValue(env, $"CANTON_LOCALNET_{slot}_JSON_API_URL")
            ?? (honourLegacyGlobals ? GetValue(env, JsonApiUrlEnv) : null)
            ?? DefaultJsonApiUrl(profile);

        var tokenUrl = GetValue(env, $"CANTON_LOCALNET_{slot}_TOKEN_URL")
            ?? (honourLegacyGlobals ? GetValue(env, TokenUrlEnv) : null)
            ?? DefaultTokenUrl(profile);

        var audience = GetValue(env, $"CANTON_LOCALNET_{slot}_AUDIENCE")
            ?? (honourLegacyGlobals ? GetValue(env, AudienceEnv) : null)
            ?? DefaultAudience;

        var scope = GetValue(env, $"CANTON_LOCALNET_{slot}_SCOPE")
            ?? (honourLegacyGlobals ? GetValue(env, ScopeEnv) : null)
            ?? DefaultScope;

        var clientId = GetValue(env, $"CANTON_LOCALNET_{slot}_CLIENT_ID")
            ?? (honourLegacyGlobals ? GetValue(env, ClientIdEnv) : null)
            ?? DefaultClientId(profile)
            ?? throw MissingCredential(profile, "CLIENT_ID");

        var clientSecret = GetValue(env, $"CANTON_LOCALNET_{slot}_CLIENT_SECRET")
            ?? (honourLegacyGlobals ? GetValue(env, ClientSecretEnv) : null)
            ?? DefaultClientSecret(profile)
            ?? throw MissingCredential(profile, "CLIENT_SECRET");

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
    /// <see cref="LocalnetProfile.AValidator1"/>.
    /// </summary>
    public static LocalnetProfile ResolveProfile(IReadOnlyDictionary<string, string?>? environment = null)
    {
        var env = environment ?? Snapshot();
        var raw = GetValue(env, ProfileEnv);
        if (string.IsNullOrWhiteSpace(raw))
        {
            return LocalnetProfile.AValidator1;
        }

        return raw.Trim().ToLowerInvariant() switch
        {
            "b-validator-1" => LocalnetProfile.BValidator1,
            "a-validator-1" => LocalnetProfile.AValidator1,
            "c-validator-1" => LocalnetProfile.CValidator1,
            "sv-validator-1" or "super-validator" or "supervalidator" => LocalnetProfile.SvValidator1,
            "d-validator-1" => LocalnetProfile.DValidator1,
            _ => throw new InvalidOperationException(
                $"Unknown profile '{raw}' in {ProfileEnv}; expected one of: a-validator-1, b-validator-1, c-validator-1, sv-validator-1, d-validator-1."),
        };
    }

    private static string DefaultJsonApiUrl(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.BValidator1 => "http://localhost:12975",
        LocalnetProfile.AValidator1 => "http://localhost:11975",
        LocalnetProfile.CValidator1 => "http://localhost:13975",
        LocalnetProfile.SvValidator1 => "http://localhost:10975",
        LocalnetProfile.DValidator1 => "http://localhost:14975",
        _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
    };

    private static string DefaultTokenUrl(LocalnetProfile profile)
    {
        var realm = profile switch
        {
            LocalnetProfile.BValidator1 => "BValidator1",
            LocalnetProfile.AValidator1 => "AValidator1",
            LocalnetProfile.CValidator1 => "CValidator1",
            LocalnetProfile.SvValidator1 => "sv-validator-1",
            LocalnetProfile.DValidator1 => "DValidator1",
            _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
        };
        return $"{DefaultKeycloakHost}/realms/{realm}/protocol/openid-connect/token";
    }

    private static string? DefaultClientId(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.AValidator1 => "a-validator-1-validator",
        LocalnetProfile.BValidator1 => "b-validator-1-validator",
        LocalnetProfile.CValidator1 => "c-validator-1-validator",
        LocalnetProfile.DValidator1 => "d-validator-1-validator",
        LocalnetProfile.SvValidator1 => "sv-validator",
        _ => null,
    };

    private static string? DefaultClientSecret(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.AValidator1 => AValidator1DemoClientSecret,
        LocalnetProfile.BValidator1 => BcdValidator1DemoClientSecret,
        LocalnetProfile.CValidator1 => BcdValidator1DemoClientSecret,
        LocalnetProfile.DValidator1 => BcdValidator1DemoClientSecret,
        LocalnetProfile.SvValidator1 => null,
        _ => null,
    };

    private static string SlotEnvPrefix(LocalnetProfile profile) => profile switch
    {
        LocalnetProfile.AValidator1 => "A_VALIDATOR_1",
        LocalnetProfile.BValidator1 => "B_VALIDATOR_1",
        LocalnetProfile.CValidator1 => "C_VALIDATOR_1",
        LocalnetProfile.DValidator1 => "D_VALIDATOR_1",
        LocalnetProfile.SvValidator1 => "SV_VALIDATOR_1",
        _ => throw new ArgumentOutOfRangeException(nameof(profile), profile, null),
    };

    private static InvalidOperationException MissingCredential(LocalnetProfile profile, string what)
    {
        var slot = SlotEnvPrefix(profile);
        return new InvalidOperationException(
            $"Environment variable 'CANTON_LOCALNET_{slot}_{what}' is required to talk to the LocalNet token endpoint for {profile}.");
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
